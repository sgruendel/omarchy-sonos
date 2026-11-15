package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"omarchy-sonos/sonos"
)

// maxConcurrentProbes bounds speaker probes per discovery.
const maxConcurrentProbes = 4

type topologyResult struct {
	rooms    []sonos.Speaker
	complete bool
}

// discoverRooms tries SSDP when cached locations cannot supply a complete
// topology. Equal-size partial results prefer the cached locations.
func (a *App) discoverRooms(ctx context.Context, locations []string, ssdp func(context.Context, time.Duration) []string) []sonos.Speaker {
	cached := a.probeLocations(ctx, locations)
	if cached.complete {
		return cached.rooms
	}
	if ctx.Err() != nil {
		return nil
	}
	found := a.probeLocations(ctx, ssdp(ctx, 3*time.Second))
	if ctx.Err() != nil {
		return nil
	}
	if found.complete || len(found.rooms) > len(cached.rooms) {
		return found.rooms
	}
	return cached.rooms
}

// probeLocations limits network load and cancels other probes after one speaker
// supplies a complete topology. If none does, it returns the largest partial
// result, preferring earlier locations on ties. All workers exit before return.
func (a *App) probeLocations(ctx context.Context, locations []string) topologyResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// A buffered channel bounds concurrency rather than errgroup.SetLimit, which
	// would make golang.org/x/sync the backend's only runtime dependency (the
	// README promises none). No error needs propagating: a failed probe simply
	// contributes no rooms.
	slots := make(chan struct{}, maxConcurrentProbes)
	result := make(chan []sonos.Speaker, 1)
	partial := make([][]sonos.Speaker, len(locations))
	var workers sync.WaitGroup

launch:
	for i, location := range locations {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			break launch
		}
		workers.Go(func() {
			defer func() { <-slots }()
			if ctx.Err() != nil {
				return
			}
			speaker, err := a.Sonos.Describe(ctx, location)
			if err != nil {
				return
			}
			rooms, err := a.Sonos.Topology(ctx, speaker)
			if errors.Is(err, sonos.ErrIncompleteTopology) {
				// Each worker owns a separate slot, read only after workers.Wait.
				partial[i] = rooms
				return
			}
			if err != nil || len(rooms) == 0 {
				return
			}
			select {
			case result <- rooms:
				cancel()
			default:
			}
		})
	}
	workers.Wait()
	select {
	case rooms := <-result:
		return topologyResult{rooms: rooms, complete: true}
	default:
	}
	if ctx.Err() != nil {
		return topologyResult{}
	}
	var best []sonos.Speaker
	for _, rooms := range partial {
		if len(rooms) > len(best) {
			best = rooms
		}
	}
	return topologyResult{rooms: best}
}
