package main

import (
	"context"
	"sync"

	"omarchy-sonos/sonos"
)

// probeLocations limits network load and cancels other probes after one speaker
// supplies a complete topology. All workers exit before the caller uses it.
func (a *App) probeLocations(ctx context.Context, locations []string) []sonos.Speaker {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	slots := make(chan struct{}, 4)
	result := make(chan []sonos.Speaker, 1)
	var workers sync.WaitGroup

launch:
	for _, location := range locations {
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
		return rooms
	default:
		return nil
	}
}
