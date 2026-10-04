package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"time"

	"omarchy-sonos/rp"
	"omarchy-sonos/sonos"
	"omarchy-sonos/storage"
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

func emptyRP() RadioParadise {
	return RadioParadise{
		Channel:  -1,
		Override: -1,
		Comments: rp.Comments{Items: []rp.Comment{}},
	}
}

type Snapshot struct {
	Type           string          `json:"type"`
	Version        int             `json:"version"`
	BackendVersion string          `json:"backendVersion"`
	RPMixes        []rp.Mix        `json:"rpMixes"`
	Status         string          `json:"status"`
	Error          string          `json:"error"`
	Rooms          []sonos.Speaker `json:"rooms"`
	Selected       string          `json:"selected"`
	Playback       sonos.Playback  `json:"playback"`
	Volume         int             `json:"volume"`
	Mute           bool            `json:"mute"`
	Account        Account         `json:"account"`
	RP             RadioParadise   `json:"rp"`
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
	startupWarning  string
}

func newApp(dir string, hosts []string) (*App, error) {
	version, err := manifestVersion()
	if err != nil {
		return nil, err
	}
	a := &App{
		Sonos:           sonos.New(),
		RP:              rp.New(filepath.Join(dir, "rp-session.json")),
		StatePath:       filepath.Join(dir, "state.json"),
		playlistChannel: -1,
		ratings:         map[int64]int{},
	}
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
		// Decoder errors may include private data. Keep diagnostics generic.
		a.startupWarning = "Could not load RP session; continuing signed out. Sign in again to replace it."
	}
	a.RP.UserAgent = "omarchy-sonos/" + version
	a.Snapshot = Snapshot{
		Type:           "snapshot",
		Version:        1,
		BackendVersion: version,
		RPMixes:        rp.Mixes(),
		Status:         "starting",
		Rooms:          []sonos.Speaker{},
		RP:             emptyRP(),
	}
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
	rooms := a.probeLocations(ctx, locations)
	if len(rooms) == 0 && ctx.Err() == nil {
		rooms = a.probeLocations(ctx, sonos.Discover(ctx, 3*time.Second))
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
	interval := time.Minute
	if a.Snapshot.Status == "offline" {
		interval = 10 * time.Second
	}
	if force || time.Since(a.lastDiscovery) >= interval {
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
		a.Snapshot.RP = emptyRP()
		if a.Snapshot.Error == "" {
			a.Snapshot.Error = err.Error()
		}
		return
	}
	p, err := a.Sonos.Playback(ctx, coordinator)
	if err != nil {
		a.Snapshot.Status = "offline"
		a.Snapshot.Error = err.Error()
		a.Snapshot.Playback = sonos.Playback{Actions: []string{}}
		a.Snapshot.RP = emptyRP()
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
	next := emptyRP()
	next.Detected = detected
	next.Channel = channel
	next.ChannelName = rp.Channels[channel]
	next.Override = override
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
