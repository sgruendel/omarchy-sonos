// Package sonos implements the local Sonos UPnP protocol without cloud credentials.
package sonos

import (
	"bytes"
	"cmp"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Type    string `xml:"serviceType"`
	Control string `xml:"controlURL"`
}
type Device struct {
	Services []Service `xml:"serviceList>service"`
	Devices  []Device  `xml:"deviceList>device"`
	Name     string    `xml:"friendlyName"`
	UDN      string    `xml:"UDN"`
}
type Speaker struct {
	UID         string             `json:"uid"`
	Name        string             `json:"name"`
	URL         string             `json:"-"`
	Coordinator string             `json:"coordinator"`
	Services    map[string]Service `json:"-"`
}

// TransportState is the UPnP AVTransport CurrentTransportState of a speaker.
type TransportState string

const (
	StatePlaying        TransportState = "PLAYING"
	StatePausedPlayback TransportState = "PAUSED_PLAYBACK"
	StateStopped        TransportState = "STOPPED"
	StateTransitioning  TransportState = "TRANSITIONING"
	StateNoMediaPresent TransportState = "NO_MEDIA_PRESENT"
)

// TransportAction is a UPnP AVTransport action. Speakers advertise the currently
// allowed ones (which may include actions not listed here, such as Seek).
type TransportAction string

const (
	ActionPlay     TransportAction = "Play"
	ActionPause    TransportAction = "Pause"
	ActionNext     TransportAction = "Next"
	ActionPrevious TransportAction = "Previous"
)

type Playback struct {
	State         TransportState    `json:"state"`
	Title         string            `json:"title"`
	Artist        string            `json:"artist"`
	Album         string            `json:"album"`
	Artwork       string            `json:"artwork"`
	URI           string            `json:"uri"`
	TrackURI      string            `json:"trackUri"`
	Station       string            `json:"station"`
	StreamContent string            `json:"streamContent"`
	Position      int               `json:"position"`
	Duration      int               `json:"duration"`
	Actions       []TransportAction `json:"actions"`
}
type Client struct{ HTTP *http.Client }

func New() *Client { return &Client{HTTP: &http.Client{Timeout: 4 * time.Second}} }

func (c *Client) Describe(ctx context.Context, location string) (Speaker, error) {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return Speaker{}, errors.New("invalid speaker URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Speaker{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Speaker{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Speaker{}, fmt.Errorf("speaker description: HTTP %d", resp.StatusCode)
	}
	var desc struct {
		Device Device `xml:"device"`
	}
	if err = xml.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&desc); err != nil {
		return Speaker{}, err
	}
	s := Speaker{UID: strings.TrimPrefix(desc.Device.UDN, "uuid:"), Name: desc.Device.Name, URL: u.Scheme + "://" + u.Host, Services: map[string]Service{}}
	var walk func(Device)
	walk = func(d Device) {
		for _, v := range d.Services {
			parts := strings.Split(v.Type, ":")
			if len(parts) > 1 {
				s.Services[parts[len(parts)-2]] = v
			}
		}
		for _, child := range d.Devices {
			walk(child)
		}
	}
	walk(desc.Device)
	if _, ok := s.Services["AVTransport"]; !ok {
		return Speaker{}, errors.New("device is not a Sonos renderer")
	}
	return s, nil
}

// Values extracts text by local XML name, including escaped DIDL/topology
// payloads. When several elements share a name, the first one with text wins:
// DIDL may repeat elements such as albumArtURI, and joining them would corrupt
// the value.
func Values(data []byte) (map[string]string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	out := map[string]string{}
	type element struct {
		name string
		text strings.Builder
	}
	var stack []*element
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := t.(type) {
		case xml.StartElement:
			stack = append(stack, &element{name: v.Name.Local})
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			// Text may arrive in several tokens (e.g. around CDATA), so store it
			// once the element ends.
			e := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, seen := out[e.name]; !seen && e.text.Len() > 0 {
				out[e.name] = e.text.String()
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(v)
			}
		}
	}
	return out, nil
}
func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func (c *Client) Call(ctx context.Context, s Speaker, service, action string, args map[string]string) (map[string]string, error) {
	svc, ok := s.Services[service]
	if !ok {
		return nil, fmt.Errorf("speaker does not support %s", service)
	}
	base, _ := url.Parse(s.URL)
	path, err := url.Parse(svc.Control)
	if err != nil {
		return nil, err
	}
	target := base.ResolveReference(path)
	if target.Host != base.Host {
		return nil, errors.New("speaker control URL leaves its host")
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + ` xmlns:u="` + escape(svc.Type) + `">`)
	keys := slices.Collect(maps.Keys(args))
	// UPnP arguments follow the order in the service action declaration.
	order := map[string]int{"InstanceID": 1, "Channel": 2, "Speed": 3, "DesiredVolume": 4, "DesiredMute": 4}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(order[a], order[b]), cmp.Compare(a, b))
	})
	for _, k := range keys {
		body.WriteString("<" + k + ">" + escape(args[k]) + "</" + k + ">")
	}
	body.WriteString("</u:" + action + "></s:Body></s:Envelope>")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(body.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+svc.Type+"#"+action+`"`)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Keep the cause (timeout, refused, ...) but not *url.Error's method and
		// URL: the action already names the request and the UI shows this text.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("Sonos %s request failed: %w", action, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	values, err := Values(data)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || values["errorCode"] != "" {
		return nil, fmt.Errorf("Sonos %s: HTTP %d, UPnP %s %s", action, resp.StatusCode, values["errorCode"], values["errorDescription"])
	}
	return values, nil
}

// maxConcurrentDescribes bounds device description requests per topology.
const maxConcurrentDescribes = 8

// ErrIncompleteTopology indicates that some visible rooms could not be described.
// Topology still returns the reachable rooms alongside this error.
var ErrIncompleteTopology = errors.New("Sonos topology contains unreachable rooms")

// Topology returns visible rooms, sorted by name. An incomplete result includes
// the reachable rooms and ErrIncompleteTopology.
func (c *Client) Topology(ctx context.Context, s Speaker) ([]Speaker, error) {
	v, err := c.Call(ctx, s, "ZoneGroupTopology", "GetZoneGroupState", nil)
	if err != nil {
		return nil, err
	}
	var state struct {
		Groups []struct {
			Coordinator string `xml:"Coordinator,attr"`
			Members     []struct {
				UID       string `xml:"UUID,attr"`
				Name      string `xml:"ZoneName,attr"`
				Location  string `xml:"Location,attr"`
				Invisible string `xml:"Invisible,attr"`
			} `xml:"ZoneGroupMember"`
		} `xml:"ZoneGroups>ZoneGroup"`
	}
	if err = xml.Unmarshal([]byte(v["ZoneGroupState"]), &state); err != nil {
		return nil, err
	}
	var members []Speaker
	var locations []string
	for _, g := range state.Groups {
		for _, m := range g.Members {
			if m.Invisible != "1" {
				members = append(members, Speaker{UID: m.UID, Name: m.Name, Coordinator: g.Coordinator})
				locations = append(locations, m.Location)
			}
		}
	}
	// Describe members concurrently: with serial requests, each unreachable room
	// adds a full client timeout and a few of them exhaust the caller's deadline.
	described := make([]bool, len(members))
	slots := make(chan struct{}, maxConcurrentDescribes)
	var workers sync.WaitGroup
	for i := range members {
		workers.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			sp, err := c.Describe(ctx, locations[i])
			if err != nil {
				return
			}
			sp.UID, sp.Name, sp.Coordinator = members[i].UID, members[i].Name, members[i].Coordinator
			// Each worker owns a separate index, read only after workers.Wait.
			members[i], described[i] = sp, true
		})
	}
	workers.Wait()
	var out []Speaker
	incomplete := false
	for i, sp := range members {
		if !described[i] {
			incomplete = true
			continue
		}
		out = append(out, sp)
	}
	if len(out) == 0 {
		return nil, errors.New("Sonos topology contains no reachable rooms")
	}
	slices.SortFunc(out, func(a, b Speaker) int { return cmp.Compare(a.Name, b.Name) })
	if incomplete {
		return out, ErrIncompleteTopology
	}
	return out, nil
}
func seconds(s string) int {
	var h, m, sec int
	if _, err := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec); err != nil {
		return 0
	}
	return h*3600 + m*60 + sec
}
func (c *Client) Playback(ctx context.Context, s Speaker) (Playback, error) {
	p := Playback{Actions: []TransportAction{}}
	args := map[string]string{"InstanceID": "0"}
	transport, err := c.Call(ctx, s, "AVTransport", "GetTransportInfo", args)
	if err != nil {
		return p, err
	}
	p.State = TransportState(transport["CurrentTransportState"])
	pos, err := c.Call(ctx, s, "AVTransport", "GetPositionInfo", args)
	if err != nil {
		return p, err
	}
	p.URI = pos["TrackURI"]
	p.TrackURI = pos["TrackURI"]
	p.Position = seconds(pos["RelTime"])
	p.Duration = seconds(pos["TrackDuration"])
	didl, _ := Values([]byte(pos["TrackMetaData"]))
	p.Title = didl["title"]
	p.Artist = didl["creator"]
	p.Album = didl["album"]
	p.StreamContent = didl["streamContent"]
	p.Artwork = didl["albumArtURI"]
	media, err := c.Call(ctx, s, "AVTransport", "GetMediaInfo", args)
	if err != nil {
		return p, err
	}
	if media["CurrentURI"] != "" {
		p.URI = media["CurrentURI"]
	}
	station, _ := Values([]byte(media["CurrentURIMetaData"]))
	p.Station = station["title"]
	if p.StreamContent != "" && p.StreamContent != "ZPSTR_BUFFERING" && p.StreamContent != "ZPSTR_CONNECTING" {
		p.Title = p.StreamContent
		p.Artist = ""
		for _, sep := range []string{" - ", " – ", " — "} {
			if a, b, ok := strings.Cut(p.StreamContent, sep); ok {
				p.Artist = a
				p.Title = b
				break
			}
		}
	}
	if p.Artwork != "" {
		base, _ := url.Parse(s.URL)
		art, e := url.Parse(p.Artwork)
		p.Artwork = ""
		if e == nil {
			resolved := base.ResolveReference(art)
			if (resolved.Scheme == "http" || resolved.Scheme == "https") && resolved.Host != "" {
				p.Artwork = resolved.String()
			}
		}
	}
	actions, err := c.Call(ctx, s, "AVTransport", "GetCurrentTransportActions", args)
	if err == nil {
		for _, a := range strings.Split(actions["Actions"], ",") {
			if a = strings.TrimSpace(a); a != "" {
				p.Actions = append(p.Actions, TransportAction(a))
			}
		}
	}
	return p, nil
}
func (c *Client) Volume(ctx context.Context, s Speaker) (int, bool, error) {
	args := map[string]string{"InstanceID": "0", "Channel": "Master"}
	v, err := c.Call(ctx, s, "RenderingControl", "GetVolume", args)
	if err != nil {
		return 0, false, err
	}
	m, err := c.Call(ctx, s, "RenderingControl", "GetMute", args)
	volume, _ := strconv.Atoi(v["CurrentVolume"])
	return volume, m["CurrentMute"] == "1", err
}
func (c *Client) SetVolume(ctx context.Context, s Speaker, volume int) error {
	if volume < 0 || volume > 100 {
		return errors.New("volume must be between 0 and 100")
	}
	_, err := c.Call(ctx, s, "RenderingControl", "SetVolume", map[string]string{"InstanceID": "0", "Channel": "Master", "DesiredVolume": strconv.Itoa(volume)})
	return err
}
func (c *Client) SetMute(ctx context.Context, s Speaker, mute bool) error {
	v := "0"
	if mute {
		v = "1"
	}
	_, err := c.Call(ctx, s, "RenderingControl", "SetMute", map[string]string{"InstanceID": "0", "Channel": "Master", "DesiredMute": v})
	return err
}
func (c *Client) Transport(ctx context.Context, s Speaker, action TransportAction) error {
	args := map[string]string{"InstanceID": "0"}
	if action == ActionPlay {
		args["Speed"] = "1"
	}
	_, err := c.Call(ctx, s, "AVTransport", string(action), args)
	return err
}
