package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeLocationsCancelsStaleHostAfterSuccess(t *testing.T) {
	a, _, _, _ := fixture(t)
	base := a.Sonos.HTTP.Transport
	staleStarted := make(chan struct{})
	var staleCanceled atomic.Bool
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "192.168.1.99" {
			close(staleStarted)
			<-r.Context().Done()
			staleCanceled.Store(true)
			return nil, r.Context().Err()
		}
		select {
		case <-staleStarted:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return base.RoundTrip(r)
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	rooms := a.probeLocations(ctx, []string{"http://192.168.1.99/description", "http://192.168.1.3/description"})
	if len(rooms) != 1 || ctx.Err() != nil || !staleCanceled.Load() {
		t.Fatalf("healthy speaker blocked behind stale host: rooms=%d err=%v canceled=%v", len(rooms), ctx.Err(), staleCanceled.Load())
	}
}

func TestProbeLocationsBoundsConcurrencyAndHonorsCancellation(t *testing.T) {
	a, _, _, _ := fixture(t)
	started := make(chan struct{}, 8)
	var requests atomic.Int32
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.probeLocations(ctx, []string{
			"http://192.168.1.1/description", "http://192.168.1.2/description",
			"http://192.168.1.3/description", "http://192.168.1.4/description",
			"http://192.168.1.5/description", "http://192.168.1.6/description",
		})
	}()
	for range 4 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("discovery did not start concurrent probes")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery did not join canceled workers")
	}
	if requests.Load() != 4 {
		t.Fatalf("discovery started %d requests, want bounded batch of 4", requests.Load())
	}
}

func TestDiscoveryRetryIntervalDependsOnStatus(t *testing.T) {
	for _, tt := range []struct {
		status string
		age    time.Duration
		want   bool
	}{
		{"offline", 11 * time.Second, true},
		{"offline", 5 * time.Second, false},
		{"ready", 11 * time.Second, false},
		{"ready", 61 * time.Second, true},
	} {
		t.Run(tt.status+tt.age.String(), func(t *testing.T) {
			a, _, _, _ := fixture(t)
			a.Snapshot.Status = tt.status
			if tt.status == "offline" {
				a.Snapshot.Rooms = nil
			}
			a.lastDiscovery = time.Now().Add(-tt.age)
			previous := a.lastDiscovery
			a.Refresh(t.Context(), false)
			if a.lastDiscovery.After(previous) != tt.want {
				t.Fatalf("discovery retry = %v, want %v", a.lastDiscovery.After(previous), tt.want)
			}
			if tt.want && a.Snapshot.Status != "ready" {
				t.Fatal("successful rediscovery did not recover playback")
			}
		})
	}
}

func TestPlaybackFailureClearsStaleMetadata(t *testing.T) {
	a, _, _, _ := fixture(t)
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	a.Refresh(t.Context(), false)
	if a.Snapshot.Status != "offline" || a.Snapshot.Playback.Title != "" || a.Snapshot.RP.Song != nil || a.Snapshot.RP.CanRate {
		t.Fatal("offline snapshot retained stale playback or rating")
	}
}
