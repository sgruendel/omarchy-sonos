package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"omarchy-sonos/rp"
	"omarchy-sonos/sonos"
)

func (c Command) validate() error {
	switch c.Op {
	case OpSetVolume:
		if c.Volume == nil {
			return errors.New("volume is required")
		}
	case OpAdjustVolume:
		if c.Delta == nil {
			return errors.New("delta is required")
		}
	case OpSetMute:
		if c.Mute == nil {
			return errors.New("mute is required")
		}
	case OpRPRate, OpRPComments:
		if c.SongID == nil || *c.SongID <= 0 {
			return errors.New("a positive songId is required")
		}
		if c.Op == OpRPRate {
			if c.Rating == nil {
				return errors.New("rating is required")
			}
			if c.Room == "" {
				return errors.New("room is required")
			}
		} else if c.Offset == nil || *c.Offset < 0 {
			return errors.New("a nonnegative offset is required")
		}
	}
	return nil
}

func (a *App) Execute(ctx context.Context, c Command) error {
	if err := c.validate(); err != nil {
		return err
	}
	switch c.Op {
	case OpRefresh:
		a.lastPlaylist = time.Time{}
		a.Refresh(ctx, true)
		return nil
	case OpSelectRoom:
		if !slices.ContainsFunc(a.Snapshot.Rooms, func(s sonos.Speaker) bool { return s.UID == c.Room }) {
			return errors.New("unknown room")
		}
		a.State.Selected = c.Room
		a.lastPlaylist = time.Time{}
		return a.save()
	case OpRPLogin:
		if err := a.RP.Login(ctx, c.Username, c.Password); err != nil {
			return err
		}
		a.lastPlaylist = time.Time{}
		a.lastComments = time.Time{}
		a.ratings = map[int64]int{}
		return nil
	case OpRPLogout:
		if err := a.RP.Logout(); err != nil {
			return err
		}
		a.lastPlaylist = time.Time{}
		a.ratings = map[int64]int{}
		return nil
	case OpRPChannel:
		if c.Channel == nil {
			return errors.New("channel is required")
		}
		if *c.Channel == rp.NoChannel {
			delete(a.State.Channels, a.State.Selected)
		} else {
			if _, ok := rp.Channels[*c.Channel]; !ok {
				return errors.New("invalid RP mix")
			}
			a.State.Channels[a.State.Selected] = *c.Channel
		}
		a.lastPlaylist = time.Time{}
		return a.save()
	case OpRPRate:
		if *c.Rating < 1 || *c.Rating > 10 {
			return errors.New("rating must be between 1 and 10")
		}
		if c.Room != a.State.Selected {
			return errors.New("selected room changed; try again")
		}
		a.lastPlaylist = time.Time{}
		a.Refresh(ctx, true)
		current := a.Snapshot.RP
		if c.Room != a.State.Selected || a.Snapshot.Status != StatusReady || !current.CanRate || current.Song == nil || current.Song.ID != *c.SongID {
			return errors.New("song changed or cannot be identified; refresh before rating")
		}
		// Metadata/comments requests can take seconds. Check the speaker again immediately
		// before submission so a song transition while those requests ran cannot be rated.
		_, coordinator, err := a.targets()
		if err != nil {
			return err
		}
		latest, err := a.Sonos.Playback(ctx, coordinator)
		if err != nil || latest.State != sonos.StatePlaying || latest.URI != a.Snapshot.Playback.URI || latest.TrackURI != a.Snapshot.Playback.TrackURI || !rp.Matches(*current.Song, latest.Title, latest.Artist, latest.StreamContent) {
			return errors.New("song changed before rating; try again")
		}
		if err := a.RP.Rate(ctx, *c.SongID, *c.Rating); err != nil {
			return err
		}
		a.ratings[*c.SongID] = *c.Rating
		return nil
	case OpRPComments:
		current := a.Snapshot.RP
		if current.Song == nil || current.Song.ID != *c.SongID {
			return errors.New("song changed; reload comments")
		}
		if *c.Offset != 0 && *c.Offset != current.Comments.Offset {
			return errors.New("invalid comments page")
		}
		page, err := a.RP.Comments(ctx, *c.SongID, *c.Offset)
		if err != nil {
			return err
		}
		if *c.Offset > 0 {
			page.Items = append(current.Comments.Items, page.Items...)
		}
		a.Snapshot.RP.Comments = page
		a.Snapshot.RP.CommentsError = ""
		a.lastComments = time.Now()
		return nil
	}
	if a.Snapshot.Status != StatusReady {
		return errors.New("Sonos is offline")
	}
	room, coordinator, err := a.targets()
	if err != nil {
		return err
	}
	switch c.Op {
	case OpSetVolume:
		return a.Sonos.SetVolume(ctx, room, *c.Volume)
	case OpAdjustVolume:
		volume, _, err := a.Sonos.Volume(ctx, room)
		if err != nil {
			return err
		}
		// Bound both operands before adding; reported volume is untrusted too.
		volume = max(0, min(100, volume))
		delta := max(-100, min(100, *c.Delta))
		return a.Sonos.SetVolume(ctx, room, max(0, min(100, volume+delta)))
	case OpSetMute:
		return a.Sonos.SetMute(ctx, room, *c.Mute)
	case OpPlayPause, OpNext, OpPrevious:
		p, err := a.Sonos.Playback(ctx, coordinator)
		if err != nil {
			return err
		}
		action := sonos.ActionPlay
		switch {
		case c.Op == OpPlayPause && p.State == sonos.StatePlaying:
			action = sonos.ActionPause
		case c.Op == OpNext:
			action = sonos.ActionNext
		case c.Op == OpPrevious:
			action = sonos.ActionPrevious
		}
		if !slices.Contains(p.Actions, action) {
			return fmt.Errorf("%s is unavailable for this source", action)
		}
		return a.Sonos.Transport(ctx, coordinator, action)
	default:
		return fmt.Errorf("unknown command %q", c.Op)
	}
}
