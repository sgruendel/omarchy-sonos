package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"omarchy-sonos/sonos"
)

func topologyProbeFixture(t *testing.T, reachable map[string][]string) *App {
	t.Helper()
	a := &App{Sonos: sonos.New()}
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 {
			t.Errorf("unexpected discovery URL: %s", r.URL)
			return nil, fmt.Errorf("unexpected discovery URL")
		}
		probe, room := parts[0], parts[1]
		if r.Method == http.MethodGet {
			if room != "description" && !slices.Contains(reachable[probe], room) {
				resp := response("")
				resp.StatusCode = http.StatusServiceUnavailable
				return resp, nil
			}
			return response(`<root><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/transport</controlURL></service><service><serviceType>urn:schemas-upnp-org:service:ZoneGroupTopology:1</serviceType><controlURL>/` + probe + `/control</controlURL></service></serviceList></device></root>`), nil
		}
		state := `<ZoneGroupState><ZoneGroups><ZoneGroup Coordinator="A">`
		for _, uid := range []string{"A", "B", "C"} {
			state += fmt.Sprintf(`<ZoneGroupMember UUID="%s" ZoneName="%s" Location="http://192.168.1.3/%s/%s"/>`, uid, uid, probe, uid)
		}
		state += `</ZoneGroup></ZoneGroups></ZoneGroupState>`
		return response("<Envelope><Body><Response><ZoneGroupState>" + xmlEscape(state) + "</ZoneGroupState></Response></Body></Envelope>"), nil
	})
	return a
}

func TestProbeLocationsWaitsForCompleteTopology(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := topologyProbeFixture(t, map[string][]string{"fast": {"A"}, "slow": {"A", "B", "C"}})
		base := a.Sonos.HTTP.Transport
		release := make(chan struct{})
		a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/slow/control" {
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			return base.RoundTrip(r)
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan []sonos.Speaker, 1)
		go func() {
			done <- a.probeLocations(ctx, []string{
				"http://192.168.1.3/fast/description",
				"http://192.168.1.3/slow/description",
			})
		}()
		// The fast partial probe has finished; the complete probe is still blocked.
		synctest.Wait()
		select {
		case rooms := <-done:
			t.Fatalf("discovery returned %d rooms before the complete probe finished", len(rooms))
		default:
		}
		close(release)
		rooms := <-done
		if len(rooms) != 3 || rooms[1].UID != "B" || rooms[2].UID != "C" {
			t.Fatalf("complete topology lost rooms: %+v", rooms)
		}
	})
}

func TestProbeLocationsKeepsBestPartialTopology(t *testing.T) {
	for _, tt := range []struct {
		name   string
		first  []string
		second []string
		want   []string
	}{
		{"largest result", []string{"A"}, []string{"A", "B"}, []string{"A", "B"}},
		{"stable tie", []string{"A"}, []string{"B"}, []string{"A"}},
		{"unreachable first probe", nil, []string{"B"}, []string{"B"}},
		{"no reachable rooms", nil, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := topologyProbeFixture(t, map[string][]string{"first": tt.first, "second": tt.second})
			rooms := a.probeLocations(t.Context(), []string{
				"http://192.168.1.3/first/description",
				"http://192.168.1.3/second/description",
			})
			var got []string
			for _, room := range rooms {
				got = append(got, room.UID)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("partial rooms = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbeLocationsDiscardsPartialTopologyOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := topologyProbeFixture(t, map[string][]string{"fast": {"A"}})
		base := a.Sonos.HTTP.Transport
		a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/blocked/description" {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return base.RoundTrip(r)
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan []sonos.Speaker, 1)
		go func() {
			done <- a.probeLocations(ctx, []string{
				"http://192.168.1.3/fast/description",
				"http://192.168.1.3/blocked/description",
			})
		}()
		synctest.Wait()
		cancel()
		if rooms := <-done; len(rooms) != 0 {
			t.Fatalf("canceled discovery returned partial rooms: %+v", rooms)
		}
	})
}

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
