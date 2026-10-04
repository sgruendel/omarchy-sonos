package main

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"encoding/xml"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omarchy-sonos/rp"
	"omarchy-sonos/sonos"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

// All IO in these app tests is in-memory, including forced discovery and RP calls.
func fixture(t *testing.T) (*App, *int, *string, *bool) {
	t.Helper()
	a, err := newApp(t.TempDir(), []string{"192.168.1.3"})
	if err != nil {
		t.Fatal(err)
	}
	submissions := 0
	title := "Track"
	changeDuringMetadata := false
	description := `<root><device><friendlyName>Room</friendlyName><UDN>uuid:RINCON_A</UDN><serviceList>`
	for _, service := range []string{"AVTransport", "RenderingControl", "ZoneGroupTopology"} {
		description += `<service><serviceType>urn:schemas-upnp-org:service:` + service + `:1</serviceType><controlURL>/control</controlURL></service>`
	}
	description += `</serviceList></device></root>`
	a.Sonos.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return response(description), nil
		}
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		action := strings.TrimSuffix(strings.Split(r.Header.Get("SOAPACTION"), "#")[1], `"`)
		payload := ""
		switch action {
		case "GetZoneGroupState":
			payload = "<ZoneGroupState>" + xmlEscape(`<ZoneGroupState><ZoneGroups><ZoneGroup Coordinator="RINCON_A"><ZoneGroupMember UUID="RINCON_A" ZoneName="Room" Location="http://192.168.1.3:1400/xml/device_description.xml"/></ZoneGroup></ZoneGroups></ZoneGroupState>`) + "</ZoneGroupState>"
		case "GetTransportInfo":
			payload = "<CurrentTransportState>PLAYING</CurrentTransportState>"
		case "GetPositionInfo":
			payload = "<TrackURI>https://stream.radioparadise.com/aac-320</TrackURI><TrackMetaData>" + xmlEscape(`<DIDL-Lite><item><title>`+title+`</title><creator>Artist</creator></item></DIDL-Lite>`) + "</TrackMetaData>"
		case "GetMediaInfo":
			payload = "<CurrentURI>https://stream.radioparadise.com/aac-320</CurrentURI>"
		case "GetCurrentTransportActions":
			payload = "<Actions>Pause,Stop</Actions>"
		case "GetVolume":
			payload = "<CurrentVolume>20</CurrentVolume>"
		case "GetMute":
			payload = "<CurrentMute>0</CurrentMute>"
		default:
			t.Errorf("unexpected speaker action %s", action)
		}
		return response("<Envelope><Body><Response>" + payload + "</Response></Body></Envelope>"), nil
	})}
	a.RP.Session = rp.Session{Username: "user", UserID: "123", PasswordToken: "private-secret"}
	a.RP.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/nowplaying_list_v2022":
			if changeDuringMetadata {
				title = "New Track"
			}
			return response(`{"song":[{"song_id":"42","title":"Track","artist":"Artist","listener_rating":7.2,"user_rating":8},{"song_id":"43","title":"New Track","artist":"Artist"}]}`), nil
		case "/siteapi.php":
			return response(`{"comments":[{"username":"someone","message":"Hello"}],"total_comments":"1","more_comments":false}`), nil
		case "/api/rating":
			submissions++
			return response(`{"status":"success"}`), nil
		default:
			t.Errorf("unexpected RP path %s", r.URL.Path)
			return response(`{}`), nil
		}
	})}
	a.Refresh(t.Context(), true)
	if a.Snapshot.Status != "ready" || a.Snapshot.RP.Song == nil {
		t.Fatalf("fixture failed: %+v", a.Snapshot)
	}
	return a, &submissions, &title, &changeDuringMetadata
}
func TestRatingRevalidatesSongAndRoom(t *testing.T) {
	for _, scenario := range []string{"current", "stale ID", "changed room", "changed song", "changed during metadata"} {
		t.Run(scenario, func(t *testing.T) {
			a, submissions, title, transition := fixture(t)
			cmd := Command{Op: "rpRate", Room: "RINCON_A", SongID: new(int64(42)), Rating: new(10)}
			switch scenario {
			case "stale ID":
				cmd.SongID = new(int64(1))
			case "changed room":
				cmd.Room = "OTHER"
			case "changed song":
				*title = "New Track"
			case "changed during metadata":
				*transition = true
			}
			err := a.Execute(t.Context(), cmd)
			if scenario == "current" {
				if err != nil || *submissions != 1 {
					t.Fatalf("rating: %v, submissions %d", err, *submissions)
				}
				a.Refresh(t.Context(), false)
				if a.Snapshot.RP.Song.UserRating != 10 {
					t.Fatal("personal rating not updated")
				}
			} else {
				if err == nil || *submissions != 0 {
					t.Fatalf("unsafe rating: %v, submissions %d", err, *submissions)
				}
			}
		})
	}
}
func TestFailedMetadataAndNonRPPlaybackClearRating(t *testing.T) {
	a, _, _, _ := fixture(t)
	a.lastPlaylist = time.Time{}
	a.RP.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return response("broken-json"), nil })}
	a.Refresh(t.Context(), false)
	if a.Snapshot.RP.Song != nil || a.Snapshot.RP.CanRate || a.Snapshot.RP.Error == "" {
		t.Fatal("failed request left stale rating enabled")
	}
	a.Snapshot.Playback = sonos.Playback{URI: "https://other.example/radio", Station: "Other station"}
	a.refreshRP(t.Context())
	if a.Snapshot.RP.Detected || a.Snapshot.RP.Song != nil {
		t.Fatal("non-RP source kept RP data")
	}
}
func TestJSONProtocolAndCredentialPrivacy(t *testing.T) {
	a, submissions, _, _ := fixture(t)
	var out bytes.Buffer
	commands := strings.NewReader("{broken}\n" + `{"id":"next","op":"next"}` + "\n" + `{"id":"volume","op":"setVolume","volume":101}` + "\n" + `{"id":"duplicate","op":"rpRate","room":"RINCON_A","songId":42,"rating":1,"rating":10}` + "\n")
	if err := a.Run(t.Context(), commands, &out, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "private-secret") {
		t.Fatal("token leaked in protocol")
	}
	// Decode each line separately, as the QML service does.
	scanner := bufio.NewScanner(&out)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	results := 0
	snapshots := 0
	for scanner.Scan() {
		var message map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			t.Fatal(err)
		}
		if message["type"] == "result" {
			results++
			if message["ok"] != false {
				t.Fatal("invalid command succeeded")
			}
		} else if message["type"] == "snapshot" {
			snapshots++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if results != 4 || snapshots != 6 {
		t.Fatalf("records: %d results, %d snapshots", results, snapshots)
	}
	if *submissions != 0 {
		t.Fatal("command with duplicate rating fields submitted a rating")
	}
}
func TestStateDefaultsAndOverrides(t *testing.T) {
	a, _, _, _ := fixture(t)
	channel := 1
	if err := a.Execute(t.Context(), Command{Op: "rpChannel", Channel: &channel}); err != nil {
		t.Fatal(err)
	}
	restored, err := newApp(filepath.Dir(a.StatePath), nil)
	if err != nil || restored.State.Channels["RINCON_A"] != 1 || restored.State.Selected != "RINCON_A" {
		t.Fatal("room settings not persisted")
	}
	channel = -1
	if err = a.Execute(t.Context(), Command{Op: "rpChannel", Channel: &channel}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.State.Channels["RINCON_A"]; ok {
		t.Fatal("override not cleared")
	}
}

func TestCommentsAppendAndResetOnSongChange(t *testing.T) {
	a, _, title, _ := fixture(t)
	a.Snapshot.RP.Comments.More = true
	a.Snapshot.RP.Comments.Offset = 20
	if err := a.Execute(t.Context(), Command{Op: "rpComments", SongID: new(int64(42)), Offset: new(20)}); err != nil {
		t.Fatal(err)
	}
	if len(a.Snapshot.RP.Comments.Items) != 2 {
		t.Fatal("page did not append")
	}
	if err := a.Execute(t.Context(), Command{Op: "rpComments", SongID: new(int64(43)), Offset: new(0)}); err == nil {
		t.Fatal("accepted comments for a stale song")
	}
	*title = "New Track"
	a.Refresh(t.Context(), false)
	if a.Snapshot.RP.Song.ID != 43 || len(a.Snapshot.RP.Comments.Items) != 1 {
		t.Fatal("comments did not reset on song transition")
	}
}

func TestGroupedRoomUsesCoordinatorForPlaybackAndRoomForVolume(t *testing.T) {
	a, _, _, _ := fixture(t)
	room := a.Snapshot.Rooms[0]
	room.UID = "RINCON_B"
	room.Name = "Kitchen"
	room.URL = "http://192.168.1.4:1400"
	a.Snapshot.Rooms = append(a.Snapshot.Rooms, room)
	a.State.Selected = room.UID
	base := a.Sonos.HTTP.Transport
	volumeCalls := 0
	a.Sonos.HTTP.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		action := r.Header.Get("SOAPACTION")
		if strings.Contains(action, "RenderingControl") {
			if r.URL.Hostname() != "192.168.1.4" {
				t.Error("volume command did not target selected room")
			}
			volumeCalls++
			if strings.Contains(action, "#SetVolume") {
				body, _ := io.ReadAll(r.Body)
				args, _ := sonos.Values(body)
				if args["DesiredVolume"] != "25" {
					t.Error("wrong volume delta")
				}
				return response("<Envelope><Body/></Envelope>"), nil
			}
		} else if r.URL.Hostname() != "192.168.1.3" {
			t.Error("playback command did not target coordinator")
		}
		return base.RoundTrip(r)
	})
	a.Refresh(t.Context(), false)
	if err := a.Execute(t.Context(), Command{Op: "adjustVolume", Delta: new(5)}); err != nil {
		t.Fatal(err)
	}
	if volumeCalls != 5 {
		t.Fatalf("expected room volume reads and write, got %d", volumeCalls)
	}
}

func TestSigningOutClearsCommentsAndPersonalRating(t *testing.T) {
	a, _, _, _ := fixture(t)
	a.ratings[42] = 10
	if err := a.Execute(t.Context(), Command{Op: "rpLogout"}); err != nil {
		t.Fatal(err)
	}
	a.Refresh(t.Context(), false)
	if a.Snapshot.Account.Authenticated || a.Snapshot.RP.CanRate || a.Snapshot.RP.Song.UserRating != 0 || len(a.Snapshot.RP.Comments.Items) != 0 || a.Snapshot.RP.CommentsError == "" || len(a.ratings) != 0 {
		t.Fatal("account data remained after logout")
	}
}

func TestNativeSonosRPTrackURLEnablesRatingsWithoutOverride(t *testing.T) {
	a, _, _, _ := fixture(t)
	a.Snapshot.Playback.URI = "x-sonosapi-radio:channel%3a0%3a4%3aresume?sid=308&sn=1"
	a.Snapshot.Playback.TrackURI = "https://audio.radioparadise.stream/audio/blocks/0/x/1019/4/g/1019-4.flac"
	a.Snapshot.Playback.Station = "The Main Mix"
	a.refreshRP(t.Context())
	if !a.Snapshot.RP.Detected || a.Snapshot.RP.Channel != 0 || a.Snapshot.RP.Song == nil || !a.Snapshot.RP.CanRate {
		t.Fatalf("native RP source did not enable rating: %+v", a.Snapshot.RP)
	}
}
