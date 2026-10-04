// Package app owns serialized speaker/account mutations and the JSON Lines protocol.
package app

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"omarchy-sonos/internal/rp"
	"omarchy-sonos/internal/sonos"
	"omarchy-sonos/internal/storage"
)

type State struct {
	Selected string         `json:"selected"`
	Hosts    []string       `json:"hosts"`
	Channels map[string]int `json:"channels"`
}
type Account struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}
type RadioParadise struct {
	Detected      bool        `json:"detected"`
	Channel       int         `json:"channel"`
	ChannelName   string      `json:"channelName"`
	Override      int         `json:"override"`
	Song          *rp.Song    `json:"song"`
	Matched       bool        `json:"matched"`
	CanRate       bool        `json:"canRate"`
	Error         string      `json:"error"`
	Comments      rp.Comments `json:"comments"`
	CommentsError string      `json:"commentsError"`
}
type Snapshot struct {
	Type     string          `json:"type"`
	Version  int             `json:"version"`
	Status   string          `json:"status"`
	Error    string          `json:"error"`
	Rooms    []sonos.Speaker `json:"rooms"`
	Selected string          `json:"selected"`
	Playback sonos.Playback  `json:"playback"`
	Volume   int             `json:"volume"`
	Mute     bool            `json:"mute"`
	Account  Account         `json:"account"`
	RP       RadioParadise   `json:"rp"`
}
type Command struct {
	ID       string `json:"id"`
	Op       string `json:"op"`
	Room     string `json:"room"`
	Volume   int    `json:"volume"`
	Delta    int    `json:"delta"`
	Mute     bool   `json:"mute"`
	Channel  *int   `json:"channel"`
	SongID   int64  `json:"songId"`
	Rating   int    `json:"rating"`
	Offset   int    `json:"offset"`
	Username string `json:"username"`
	Password string `json:"password"`
}
type App struct {
	Sonos           *sonos.Client
	RP              *rp.Client
	State           State
	Snapshot        Snapshot
	StatePath       string
	lastDiscovery   time.Time
	lastPlaylist    time.Time
	playlistChannel int
	playlist        []rp.Song
	commentsSong    int64
	lastComments    time.Time
	ratings         map[int64]int
}

func New(dir string, hosts []string) (*App, error) {
	a := &App{Sonos: sonos.New(), RP: rp.New(filepath.Join(dir, "rp-session.json")), StatePath: filepath.Join(dir, "state.json"), playlistChannel: -1, ratings: map[int64]int{}}
	if err := storage.Load(a.StatePath, &a.State); err != nil {
		return nil, fmt.Errorf("could not load speaker state: %w", err)
	}
	if a.State.Channels == nil {
		a.State.Channels = map[string]int{}
	}
	for _, h := range hosts {
		if _, err := sonos.HostLocation(h); err != nil {
			return nil, err
		}
		if !slices.Contains(a.State.Hosts, h) {
			a.State.Hosts = append(a.State.Hosts, h)
		}
	}
	if err := a.RP.Load(); err != nil {
		return nil, errors.New("could not load RP session")
	}
	a.Snapshot = Snapshot{Type: "snapshot", Version: 1, Status: "starting", Rooms: []sonos.Speaker{}, RP: RadioParadise{Channel: -1, Override: -1, Comments: rp.Comments{Items: []rp.Comment{}}}}
	return a, nil
}
func (a *App) save() error { return storage.Save(a.StatePath, a.State) }
func (a *App) discover(ctx context.Context) error {
	a.lastDiscovery = time.Now()
	var locations []string
	for _, h := range a.State.Hosts {
		loc, err := sonos.HostLocation(h)
		if err == nil {
			locations = append(locations, loc)
		}
	}
	probe := func(locs []string) []sonos.Speaker {
		for _, loc := range locs {
			sp, err := a.Sonos.Describe(ctx, loc)
			if err != nil {
				continue
			}
			rooms, err := a.Sonos.Topology(ctx, sp)
			if err == nil {
				return rooms
			}
		}
		return nil
	}
	rooms := probe(locations)
	if len(rooms) == 0 {
		rooms = probe(sonos.Discover(ctx, 3*time.Second))
	}
	if len(rooms) == 0 {
		return errors.New("no Sonos speakers found; check the LAN or configure SONOS_HOSTS")
	}
	a.Snapshot.Rooms = rooms
	for _, sp := range rooms {
		u, _ := url.Parse(sp.URL)
		host := u.Hostname()
		if !slices.Contains(a.State.Hosts, host) {
			a.State.Hosts = append(a.State.Hosts, host)
		}
	}
	if !slices.ContainsFunc(rooms, func(s sonos.Speaker) bool { return s.UID == a.State.Selected }) {
		a.State.Selected = rooms[0].UID
	}
	return a.save()
}
func (a *App) targets() (sonos.Speaker, sonos.Speaker, error) {
	var room sonos.Speaker
	for _, s := range a.Snapshot.Rooms {
		if s.UID == a.State.Selected {
			room = s
			break
		}
	}
	if room.UID == "" {
		return room, room, errors.New("select a reachable Sonos room")
	}
	for _, s := range a.Snapshot.Rooms {
		if s.UID == room.Coordinator {
			return room, s, nil
		}
	}
	return room, room, errors.New("group coordinator is unreachable")
}
func (a *App) Refresh(ctx context.Context, force bool) {
	a.Snapshot.Error = ""
	a.Snapshot.Account = Account{Authenticated: a.RP.Authenticated(), Username: a.RP.Session.Username}
	if force || time.Since(a.lastDiscovery) > 60*time.Second {
		if err := a.discover(ctx); err != nil {
			a.Snapshot.Rooms = []sonos.Speaker{}
			a.Snapshot.Status = "offline"
			a.Snapshot.Error = err.Error()
		}
	}
	a.Snapshot.Selected = a.State.Selected
	room, coordinator, err := a.targets()
	if err != nil {
		a.Snapshot.Status = "offline"
		a.Snapshot.Playback = sonos.Playback{Actions: []string{}}
		a.Snapshot.RP = RadioParadise{Channel: -1, Override: -1, Comments: rp.Comments{Items: []rp.Comment{}}}
		return
	}
	p, err := a.Sonos.Playback(ctx, coordinator)
	if err != nil {
		a.Snapshot.Status = "offline"
		a.Snapshot.Error = err.Error()
		a.Snapshot.RP = RadioParadise{Channel: -1, Override: -1, Comments: rp.Comments{Items: []rp.Comment{}}}
		return
	}
	a.Snapshot.Playback = p
	a.Snapshot.Status = "ready"
	a.Snapshot.Volume, a.Snapshot.Mute, err = a.Sonos.Volume(ctx, room)
	if err != nil {
		a.Snapshot.Error = err.Error()
	}
	a.refreshRP(ctx)
}
func (a *App) refreshRP(ctx context.Context) {
	p := a.Snapshot.Playback
	// Media URI can be an opaque Sonos service/queue URL. The track URI retains
	// the audio provider and RP mix even when the station title is just "The Main Mix".
	detected, channel := rp.Detect(p.TrackURI, p.Station)
	if !detected {
		detected, channel = rp.Detect(p.URI, p.Station)
	}
	override := -1
	if ch, ok := a.State.Channels[a.State.Selected]; ok {
		override = ch
	}
	if override >= 0 {
		detected = true
		channel = override
	}
	next := RadioParadise{Detected: detected, Channel: channel, ChannelName: rp.Channels[channel], Override: override, Comments: rp.Comments{Items: []rp.Comment{}}}
	defer func() { a.Snapshot.RP = next }()
	if !detected {
		return
	}
	if channel < 0 {
		next.Error = "Select the RP mix below to identify this stream"
		return
	}
	if a.playlistChannel != channel || time.Since(a.lastPlaylist) > 15*time.Second {
		// Failed requests clear identification; stale cached data must never enable rating.
		a.playlist = nil
		a.lastPlaylist = time.Now()
		a.playlistChannel = channel
		songs, err := a.RP.Playlist(ctx, channel)
		if err != nil {
			next.Error = err.Error()
			return
		}
		a.playlist = songs
	}
	song := rp.Match(a.playlist, p.Title, p.Artist, p.StreamContent)
	if song == nil {
		next.Error = "Waiting for Sonos track metadata to match the RP playlist"
		return
	}
	if value, ok := a.ratings[song.ID]; ok {
		song.UserRating = value
	}
	next.Song = song
	next.Matched = true
	next.CanRate = p.State == "PLAYING" && a.RP.Authenticated()
	if !a.RP.Authenticated() {
		next.CommentsError = rp.ErrCommentsAuth.Error()
		return
	}
	if a.commentsSong != song.ID {
		a.commentsSong = song.ID
		a.lastComments = time.Time{}
	}
	if a.Snapshot.RP.Song != nil && a.Snapshot.RP.Song.ID == song.ID {
		next.Comments = a.Snapshot.RP.Comments
		next.CommentsError = a.Snapshot.RP.CommentsError
	}
	if time.Since(a.lastComments) > 2*time.Minute {
		a.lastComments = time.Now()
		page, err := a.RP.Comments(ctx, song.ID, 0)
		if err != nil {
			next.CommentsError = err.Error()
		} else {
			next.Comments = page
			next.CommentsError = ""
		}
	}
}
func (a *App) Execute(ctx context.Context, c Command) error {
	switch c.Op {
	case "refresh":
		a.lastPlaylist = time.Time{}
		a.Refresh(ctx, true)
		return nil
	case "selectRoom":
		if !slices.ContainsFunc(a.Snapshot.Rooms, func(s sonos.Speaker) bool { return s.UID == c.Room }) {
			return errors.New("unknown room")
		}
		a.State.Selected = c.Room
		a.lastPlaylist = time.Time{}
		return a.save()
	case "rpLogin":
		if err := a.RP.Login(ctx, c.Username, c.Password); err != nil {
			return err
		}
		a.lastPlaylist = time.Time{}
		a.lastComments = time.Time{}
		a.ratings = map[int64]int{}
		return nil
	case "rpLogout":
		if err := a.RP.Logout(); err != nil {
			return err
		}
		a.lastPlaylist = time.Time{}
		a.ratings = map[int64]int{}
		return nil
	case "rpChannel":
		if c.Channel == nil {
			return errors.New("channel is required")
		}
		if *c.Channel == -1 {
			delete(a.State.Channels, a.State.Selected)
		} else {
			if _, ok := rp.Channels[*c.Channel]; !ok {
				return errors.New("invalid RP mix")
			}
			a.State.Channels[a.State.Selected] = *c.Channel
		}
		a.lastPlaylist = time.Time{}
		return a.save()
	case "rpRate":
		if c.Rating < 1 || c.Rating > 10 {
			return errors.New("rating must be between 1 and 10")
		}
		if c.Room != a.State.Selected {
			return errors.New("selected room changed; try again")
		}
		a.lastPlaylist = time.Time{}
		a.Refresh(ctx, true)
		current := a.Snapshot.RP
		if c.Room != a.State.Selected || a.Snapshot.Status != "ready" || !current.CanRate || current.Song == nil || current.Song.ID != c.SongID {
			return errors.New("song changed or cannot be identified; refresh before rating")
		}
		// Metadata/comments requests can take seconds. Check the speaker again immediately
		// before submission so a song transition while those requests ran cannot be rated.
		_, coordinator, err := a.targets()
		if err != nil {
			return err
		}
		latest, err := a.Sonos.Playback(ctx, coordinator)
		if err != nil || latest.State != "PLAYING" || latest.URI != a.Snapshot.Playback.URI || latest.TrackURI != a.Snapshot.Playback.TrackURI || !rp.Matches(*current.Song, latest.Title, latest.Artist, latest.StreamContent) {
			return errors.New("song changed before rating; try again")
		}
		if err := a.RP.Rate(ctx, c.SongID, c.Rating); err != nil {
			return err
		}
		a.ratings[c.SongID] = c.Rating
		return nil
	case "rpComments":
		current := a.Snapshot.RP
		if current.Song == nil || current.Song.ID != c.SongID {
			return errors.New("song changed; reload comments")
		}
		if c.Offset != 0 && c.Offset != current.Comments.Offset {
			return errors.New("invalid comments page")
		}
		page, err := a.RP.Comments(ctx, c.SongID, c.Offset)
		if err != nil {
			return err
		}
		if c.Offset > 0 {
			page.Items = append(current.Comments.Items, page.Items...)
		}
		a.Snapshot.RP.Comments = page
		a.Snapshot.RP.CommentsError = ""
		a.lastComments = time.Now()
		return nil
	}
	if a.Snapshot.Status != "ready" {
		return errors.New("Sonos is offline")
	}
	room, coordinator, err := a.targets()
	if err != nil {
		return err
	}
	switch c.Op {
	case "setVolume":
		return a.Sonos.SetVolume(ctx, room, c.Volume)
	case "adjustVolume":
		volume, _, err := a.Sonos.Volume(ctx, room)
		if err != nil {
			return err
		}
		return a.Sonos.SetVolume(ctx, room, max(0, min(100, volume+c.Delta)))
	case "setMute":
		return a.Sonos.SetMute(ctx, room, c.Mute)
	case "playPause", "next", "previous":
		p, err := a.Sonos.Playback(ctx, coordinator)
		if err != nil {
			return err
		}
		action := "Play"
		if c.Op == "playPause" && p.State == "PLAYING" {
			action = "Pause"
		}
		if c.Op == "next" {
			action = "Next"
		}
		if c.Op == "previous" {
			action = "Previous"
		}
		if !slices.Contains(p.Actions, action) {
			return fmt.Errorf("%s is unavailable for this source", action)
		}
		return a.Sonos.Transport(ctx, coordinator, action)
	default:
		return fmt.Errorf("unknown command %q", c.Op)
	}
}

// Run serializes all commands and snapshots; EOF terminates the owned backend process.
func (a *App) Run(ctx context.Context, in io.Reader, out io.Writer, once bool) error {
	enc := jsontext.NewEncoder(out)
	emit := func() error { return json.MarshalEncode(enc, a.Snapshot) }
	refresh := func(force bool) {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		a.Refresh(requestCtx, force)
	}
	if err := emit(); err != nil {
		return err
	}
	refresh(true)
	if err := emit(); err != nil {
		return err
	}
	if once {
		return nil
	}
	type input struct {
		line []byte
		err  error
	}
	lines := make(chan input, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- input{line: line}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case lines <- input{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			if line.err != nil {
				return line.err
			}
			var c Command
			err := json.Unmarshal(line.line, &c)
			if err != nil {
				err = errors.New("invalid command JSON")
			} else {
				requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
				err = a.Execute(requestCtx, c)
				cancel()
			}
			result := map[string]any{"type": "result", "id": c.ID, "ok": err == nil}
			if err != nil {
				result["error"] = err.Error()
			}
			if e := json.MarshalEncode(enc, result); e != nil {
				return e
			}
			if err == nil && c.Op != "refresh" {
				refresh(false)
			}
			if e := emit(); e != nil {
				return e
			}
		case <-ticker.C:
			refresh(false)
			if err := emit(); err != nil {
				return err
			}
		}
	}
}

// ParseHosts accepts comma-separated local IPs, never URLs or credentials.
func ParseHosts(value string) []string {
	var hosts []string
	for _, h := range strings.Split(value, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
