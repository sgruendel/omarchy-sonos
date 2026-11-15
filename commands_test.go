package main

import (
	"encoding/json/v2"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"omarchy-sonos/sonos"
	"omarchy-sonos/storage"
)

func TestMissingCommandFieldsDoNotPerformIO(t *testing.T) {
	for _, input := range []string{
		`{"op":"setVolume"}`,
		`{"op":"setVolume","volume":null}`,
		`{"op":"adjustVolume"}`,
		`{"op":"setMute"}`,
		`{"op":"setMute","mute":null}`,
		`{"op":"rpRate","room":"RINCON_A","rating":10}`,
		`{"op":"rpRate","room":"RINCON_A","songId":42}`,
		`{"op":"rpRate","songId":42,"rating":10}`,
		`{"op":"rpRate","room":"RINCON_A","songId":0,"rating":10}`,
		`{"op":"rpComments","offset":0}`,
		`{"op":"rpComments","songId":42}`,
		`{"op":"rpComments","songId":42,"offset":null}`,
		`{"op":"rpComments","songId":42,"offset":-1}`,
	} {
		t.Run(input, func(t *testing.T) {
			a, _, _, _ := fixture(t)
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				t.Errorf("invalid command made request to %s", r.URL.Path)
				return response(`{}`), nil
			})
			a.Sonos.HTTP.Transport = transport
			a.RP.HTTP.Transport = transport
			var c Command
			if err := json.Unmarshal([]byte(input), &c); err != nil {
				t.Fatal(err)
			}
			if err := a.Execute(t.Context(), c); err == nil {
				t.Fatal("missing or invalid field accepted")
			}
		})
	}
}

func TestExplicitZeroAndFalseCommands(t *testing.T) {
	a, _, _, _ := fixture(t)
	var actions []string
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		actions = append(actions, string(body))
		return response("<Envelope><Body/></Envelope>"), nil
	})
	for _, input := range []string{`{"op":"setVolume","volume":0}`, `{"op":"setMute","mute":false}`} {
		var c Command
		if err := json.Unmarshal([]byte(input), &c); err != nil {
			t.Fatal(err)
		}
		if err := a.Execute(t.Context(), c); err != nil {
			t.Fatal(err)
		}
	}
	if len(actions) != 2 || !strings.Contains(actions[0], "<DesiredVolume>0</DesiredVolume>") || !strings.Contains(actions[1], "<DesiredMute>0</DesiredMute>") {
		t.Fatalf("zero/false commands were not preserved: %v", actions)
	}
	for _, input := range []string{`{"op":"adjustVolume","delta":0}`, `{"op":"rpComments","songId":42,"offset":0}`} {
		var c Command
		if err := json.Unmarshal([]byte(input), &c); err != nil {
			t.Fatal(err)
		}
		if err := c.validate(); err != nil {
			t.Fatalf("explicit zero rejected: %v", err)
		}
	}
}

func TestVolumeDeltaClampsWithoutOverflow(t *testing.T) {
	for _, tt := range []struct {
		name          string
		volume, delta int
		want          string
	}{
		{"maximum delta", 20, math.MaxInt, "100"},
		{"minimum delta", 20, math.MinInt, "0"},
		{"maximum reported volume plus positive delta", math.MaxInt, 1, "100"},
		{"maximum reported volume plus negative delta", math.MaxInt, -5, "95"},
		{"minimum reported volume plus negative delta", math.MinInt, -1, "0"},
		{"minimum reported volume plus positive delta", math.MinInt, 5, "5"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, _, _, _ := fixture(t)
			base := a.Sonos.HTTP.Transport
			writes := 0
			a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.Header.Get("SOAPACTION"), "#GetVolume") {
					return response("<Envelope><Body><Response><CurrentVolume>" + strconv.Itoa(tt.volume) + "</CurrentVolume></Response></Body></Envelope>"), nil
				}
				if strings.Contains(r.Header.Get("SOAPACTION"), "#SetVolume") {
					writes++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(body), "<DesiredVolume>"+tt.want+"</DesiredVolume>") {
						t.Errorf("extreme delta produced wrong volume: %s", body)
					}
					return response("<Envelope><Body/></Envelope>"), nil
				}
				return base.RoundTrip(r)
			})
			if err := a.Execute(t.Context(), Command{Op: OpAdjustVolume, Delta: new(tt.delta)}); err != nil || writes != 1 {
				t.Fatalf("volume command: %v, writes=%d", err, writes)
			}
		})
	}
}

// speakerControls serves the given transport state and advertised actions, and
// records control requests with their arguments. Other requests reach the
// fixture's speaker.
func speakerControls(t *testing.T, a *App, state sonos.TransportState, actions string) *[]map[string]string {
	t.Helper()
	base := a.Sonos.HTTP.Transport
	var calls []map[string]string
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			return base.RoundTrip(r)
		}
		action := strings.TrimSuffix(strings.Split(r.Header.Get("SOAPACTION"), "#")[1], `"`)
		switch action {
		case "GetTransportInfo":
			return response("<Envelope><Body><Response><CurrentTransportState>" + string(state) + "</CurrentTransportState></Response></Body></Envelope>"), nil
		case "GetCurrentTransportActions":
			return response("<Envelope><Body><Response><Actions>" + actions + "</Actions></Response></Body></Envelope>"), nil
		case "Play", "Pause", "Next", "Previous":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			args, err := sonos.Values(body)
			if err != nil {
				t.Fatal(err)
			}
			args["action"] = action
			calls = append(calls, args)
			return response("<Envelope><Body/></Envelope>"), nil
		}
		return base.RoundTrip(r)
	})
	return &calls
}

func TestTransportCommandsFollowStateAndAdvertisedActions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		op      Op
		state   sonos.TransportState
		actions string
		want    sonos.TransportAction // empty: the command must fail without a request
	}{
		{"pause while playing", OpPlayPause, sonos.StatePlaying, "Play, Pause, Next, Previous", sonos.ActionPause},
		{"play while paused", OpPlayPause, sonos.StatePausedPlayback, "Play,Pause", sonos.ActionPlay},
		{"play while stopped", OpPlayPause, sonos.StateStopped, "Play,Stop", sonos.ActionPlay},
		{"play while transitioning", OpPlayPause, sonos.StateTransitioning, "Play", sonos.ActionPlay},
		{"next", OpNext, sonos.StatePlaying, "Pause,Next", sonos.ActionNext},
		{"next while paused", OpNext, sonos.StatePausedPlayback, "Play,Next", sonos.ActionNext},
		{"previous", OpPrevious, sonos.StatePlaying, "Pause,Previous", sonos.ActionPrevious},
		{"pause not advertised", OpPlayPause, sonos.StatePlaying, "Stop", ""},
		{"play not advertised", OpPlayPause, sonos.StateStopped, "Stop", ""},
		{"next not advertised", OpNext, sonos.StatePlaying, "Pause,Stop", ""},
		{"previous not advertised", OpPrevious, sonos.StatePlaying, "Pause,Next", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, _, _, _ := fixture(t)
			calls := speakerControls(t, a, tt.state, tt.actions)
			err := a.Execute(t.Context(), Command{Op: tt.op})
			if tt.want == "" {
				if err == nil || !strings.Contains(err.Error(), "unavailable") || len(*calls) != 0 {
					t.Fatalf("unadvertised action: err=%v, calls=%v", err, *calls)
				}
				return
			}
			if err != nil || len(*calls) != 1 || (*calls)[0]["action"] != string(tt.want) {
				t.Fatalf("%s: err=%v, calls=%v, want one %s", tt.op, err, *calls, tt.want)
			}
			if got := (*calls)[0]; got["InstanceID"] != "0" || (tt.want == sonos.ActionPlay) != (got["Speed"] == "1") {
				t.Fatalf("%s arguments: %v", tt.want, got)
			}
		})
	}
}

func TestSpeakerCommandsRequireReadyStatus(t *testing.T) {
	for _, op := range []Op{OpPlayPause, OpNext, OpPrevious, OpSetMute, OpSetVolume, OpAdjustVolume} {
		t.Run(string(op), func(t *testing.T) {
			a, _, _, _ := fixture(t)
			a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				t.Errorf("offline command made request to %s", r.URL.Path)
				return response(`{}`), nil
			})
			a.Snapshot.Status = StatusOffline
			err := a.Execute(t.Context(), Command{Op: op, Volume: new(10), Delta: new(5), Mute: new(true)})
			if err == nil || err.Error() != "Sonos is offline" {
				t.Fatalf("offline %s: %v", op, err)
			}
		})
	}
}

func TestSelectRoomPersistsOnlyKnownRooms(t *testing.T) {
	a, _, _, _ := fixture(t)
	if err := os.Remove(a.StatePath); err != nil {
		t.Fatal(err)
	}
	if err := a.Execute(t.Context(), Command{Op: OpSelectRoom, Room: "RINCON_UNKNOWN"}); err == nil || a.State.Selected != "RINCON_A" {
		t.Fatalf("unknown room: err=%v, selected=%q", err, a.State.Selected)
	}
	if _, err := os.Stat(a.StatePath); !os.IsNotExist(err) {
		t.Fatalf("unknown room was persisted: %v", err)
	}
	a.State.Selected = ""
	if err := a.Execute(t.Context(), Command{Op: OpSelectRoom, Room: "RINCON_A"}); err != nil {
		t.Fatal(err)
	}
	var saved State
	if err := storage.Load(a.StatePath, &saved); err != nil || saved.Selected != "RINCON_A" || !slices.Contains(saved.Hosts, "192.168.1.3") {
		t.Fatalf("selected room not persisted: %+v, %v", saved, err)
	}
}

func TestUnknownCommandIsRejected(t *testing.T) {
	a, _, _, _ := fixture(t)
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unknown command made request to %s", r.URL.Path)
		return response(`{}`), nil
	})
	if err := a.Execute(t.Context(), Command{Op: "reboot"}); err == nil || err.Error() != `unknown command "reboot"` {
		t.Fatalf("unknown command: %v", err)
	}
}
