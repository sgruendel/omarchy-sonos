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

// validate checks a command's own fields before any IO. Checks that depend on
// the app's state (selected room, current song) stay in Execute.
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
	case OpRPChannel:
		if c.Channel == nil {
			return errors.New("channel is required")
		}
		if _, ok := rp.Channels[*c.Channel]; !ok && *c.Channel != rp.NoChannel {
			return errors.New("invalid RP mix")
		}
	case OpRPRate:
		if c.SongID == nil || *c.SongID <= 0 {
			return errors.New("a positive songId is required")
		}
		if c.Rating == nil {
			return errors.New("rating is required")
		}
		if *c.Rating < 1 || *c.Rating > 10 {
			return errors.New("rating must be between 1 and 10")
		}
		if c.Room == "" {
			return errors.New("room is required")
		}
	case OpRPComments:
		if c.SongID == nil || *c.SongID <= 0 {
			return errors.New("a positive songId is required")
		}
		if c.Offset == nil || *c.Offset < 0 {
			return errors.New("a nonnegative offset is required")
		}
	}
	return nil
}

// Execute validates c and runs it. Speaker commands additionally need a ready
// snapshot; see speakerCommand.
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
		return a.selectRoom(c.Room)
	case OpRPLogin:
		return a.rpLogin(ctx, c.Username, c.Password)
	case OpRPLogout:
		return a.rpLogout()
	case OpRPChannel:
		return a.setMixOverride(*c.Channel)
	case OpRPRate:
		return a.rate(ctx, c.Room, *c.SongID, *c.Rating)
	case OpRPComments:
		return a.loadComments(ctx, *c.SongID, *c.Offset)
	case OpSetVolume, OpAdjustVolume, OpSetMute, OpPlayPause, OpNext, OpPrevious:
		return a.speakerCommand(ctx, c)
	default:
		return fmt.Errorf("unknown command %q", c.Op)
	}
}

func (a *App) selectRoom(uid string) error {
	if !slices.ContainsFunc(a.Snapshot.Rooms, func(s sonos.Speaker) bool { return s.UID == uid }) {
		return errors.New("unknown room")
	}
	a.State.Selected = uid
	a.lastPlaylist = time.Time{}
	return a.save()
}

func (a *App) rpLogin(ctx context.Context, username, password string) error {
	if err := a.RP.Login(ctx, username, password); err != nil {
		return err
	}
	a.lastPlaylist = time.Time{}
	a.lastComments = time.Time{}
	a.ratings = map[int64]int{}
	return nil
}

func (a *App) rpLogout() error {
	if err := a.RP.Logout(); err != nil {
		return err
	}
	a.lastPlaylist = time.Time{}
	a.ratings = map[int64]int{}
	return nil
}

// setMixOverride stores the RP mix for the selected room; rp.NoChannel clears it.
func (a *App) setMixOverride(channel int) error {
	// Overrides are stored per room; without one the "" key would persist.
	if a.State.Selected == "" {
		return errors.New("select a Sonos room first")
	}
	if channel == rp.NoChannel {
		delete(a.State.Channels, a.State.Selected)
	} else {
		a.State.Channels[a.State.Selected] = channel
	}
	a.lastPlaylist = time.Time{}
	return a.save()
}

// rate submits a rating only if room and song still match what the speaker
// plays, rechecked immediately before the RP request.
func (a *App) rate(ctx context.Context, room string, songID int64, rating int) error {
	if room != a.State.Selected {
		return errors.New("selected room changed; try again")
	}
	a.lastPlaylist = time.Time{}
	a.Refresh(ctx, true)
	current := a.Snapshot.RP
	if room != a.State.Selected || a.Snapshot.Status != StatusReady || !current.CanRate || current.Song == nil || current.Song.ID != songID {
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
	if err := a.RP.Rate(ctx, songID, rating); err != nil {
		return err
	}
	a.ratings[songID] = rating
	return nil
}

// loadComments loads the first page (offset 0) or appends the next advertised page.
func (a *App) loadComments(ctx context.Context, songID int64, offset int) error {
	current := a.Snapshot.RP
	if current.Song == nil || current.Song.ID != songID {
		return errors.New("song changed; reload comments")
	}
	if offset != 0 && offset != current.Comments.Offset {
		return errors.New("invalid comments page")
	}
	page, err := a.RP.Comments(ctx, songID, offset)
	if err != nil {
		return err
	}
	if offset > 0 {
		page.Items = append(current.Comments.Items, page.Items...)
	}
	a.Snapshot.RP.Comments = page
	a.Snapshot.RP.CommentsError = ""
	a.lastComments = time.Now()
	return nil
}

// speakerCommand runs a command that needs a reachable speaker. Volume and mute
// target the selected room; transport commands target its group coordinator.
func (a *App) speakerCommand(ctx context.Context, c Command) error {
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
		return a.adjustVolume(ctx, room, *c.Delta)
	case OpSetMute:
		return a.Sonos.SetMute(ctx, room, *c.Mute)
	case OpPlayPause, OpNext, OpPrevious:
		return a.transport(ctx, coordinator, c.Op)
	}
	return fmt.Errorf("unknown speaker command %q", c.Op)
}

func (a *App) adjustVolume(ctx context.Context, room sonos.Speaker, delta int) error {
	volume, _, err := a.Sonos.Volume(ctx, room)
	if err != nil {
		return err
	}
	// Bound both operands before adding; reported volume is untrusted too.
	volume = max(0, min(100, volume))
	delta = max(-100, min(100, delta))
	return a.Sonos.SetVolume(ctx, room, max(0, min(100, volume+delta)))
}

// transport plays or pauses depending on the coordinator's state, or skips,
// but only if the source currently advertises that action.
func (a *App) transport(ctx context.Context, coordinator sonos.Speaker, op Op) error {
	p, err := a.Sonos.Playback(ctx, coordinator)
	if err != nil {
		return err
	}
	action := sonos.ActionPlay
	switch {
	case op == OpPlayPause && p.State == sonos.StatePlaying:
		action = sonos.ActionPause
	case op == OpNext:
		action = sonos.ActionNext
	case op == OpPrevious:
		action = sonos.ActionPrevious
	}
	if !slices.Contains(p.Actions, action) {
		return fmt.Errorf("%s is unavailable for this source", action)
	}
	return a.Sonos.Transport(ctx, coordinator, action)
}
