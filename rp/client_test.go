package rp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectionAndMatching(t *testing.T) {
	for _, tt := range []struct {
		uri, station string
		rp           bool
		channel      int
	}{
		{"x-rincon-mp3radio://https://stream.radioparadise.com/aac-320", "", true, 0},
		{"https://stream.radioparadise.com/mellow-320", "", true, 1},
		{"https://stream.radioparadise.com/rock-320", "", true, 2},
		{"https://stream.radioparadise.com/global-flac", "", true, 3},
		{"https://stream.radioparadise.com/serenity", "", true, 42},
		{"x-sonosapi-stream:s123?sid=254", "Radio Paradise", true, -1},
		{"x-sonosapi-stream:s123?sid=254", "Radio Paradise Beyond", true, 5},
		{"https%3A%2F%2Fstream.radioparadise.com%2Faac-320", "", true, 0},
		{"https://stream.radioparadise.com/stream?chan=945", "", true, 945},
		{"https://audio.radioparadise.stream/audio/blocks/0/x/1019/4/g/1019-4.flac", "The Main Mix", true, 0},
		{"https://audio.radioparadise.stream/audio/blocks/1/x/1019/4/g/1019-4.flac", "", true, 1},
		{"https://audio.radioparadise.stream/audio/blocks/42/x/1019/4/g/1019-4.flac", "", true, 42},
		{"https://audio.radioparadise.stream.attacker.example/audio/blocks/0/x/1.flac", "", false, -1},
		{"https://radioparadise.com.attacker.example/aac-320", "Other radio", false, -1},
		{"https://example.com/radioparadise.com", "", false, -1},
		{"https://example.com/aac-320", "", false, -1},
	} {
		t.Run(tt.uri+tt.station, func(t *testing.T) {
			detected, ch := Detect(tt.uri, tt.station)
			if detected != tt.rp || ch != tt.channel {
				t.Fatalf("got %v,%d want %v,%d", detected, ch, tt.rp, tt.channel)
			}
		})
	}
	s := Song{ID: 1, Artist: "Björk", Title: "Army of Me"}
	if !Matches(s, "Army of Me", "Björk", "") || !Matches(s, "", "", "Björk – Army of Me") {
		t.Fatal("valid stream metadata did not match")
	}
	if Matches(s, "Army of Me", "Another Artist", "") || Matches(s, "Army of Me", "", "") {
		t.Fatal("ambiguous title matched")
	}
	if Match([]Song{{ID: 0, Title: s.Title, Artist: s.Artist}, s}, s.Title, s.Artist, "").ID != 1 {
		t.Fatal("accepted missing song ID")
	}
}
func TestAccountMetadataCommentsAndRating(t *testing.T) {
	ctx := t.Context()
	ratings := 0
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "omarchy-sonos/test-version" {
			t.Errorf("wrong User-Agent: %s", r.UserAgent())
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/auth" {
			for name, want := range map[string]string{"C_username": "tester", "C_passwd": "secret-token", "C_user_id": "123", "C_validated": "yes"} {
				cookie, err := r.Cookie(name)
				if err != nil || cookie.Value != want {
					t.Errorf("missing cookie %s", name)
				}
			}
		}
		switch r.URL.Path {
		case "/api/auth":
			if r.URL.Query().Get("passwd") != "password" {
				t.Error("password not sent")
			}
			_, _ = w.Write([]byte(`{"status":"success","username":"tester","passwd":"secret-token","user_id":"123"}`))
		case "/api/nowplaying_list_v2022":
			_, _ = w.Write([]byte(`{"image_base":"//img.radioparadise.com/","song":[{"song_id":"42","artist":"Artist &amp; Co","title":"Title","album":"Album","cover":"covers/l/1.jpg","listener_rating":7.8,"ratings_num":"1234","rating":"9"},{"song_id":43,"artist":"Other","title":"Other","cover":"https://img.radioparadise.com/a.jpg","listener_rating":"6.1","user_rating":8}]}`))
		case "/siteapi.php":
			if r.URL.Query().Get("file") != "comments::list" || r.URL.Query().Get("comments_offset") != "20" {
				t.Error("invalid comment request")
			}
			_, _ = w.Write([]byte(`{"comments":[{"username":"listener","posted_time":"Today","message":"<strong>Hi</strong><br/>A &amp; B<script>bad()</script>","upvotes":"3","downvotes":1}],"total_comments":"21","more_comments":false,"more_offset":21}`))
		case "/api/rating":
			ratings++
			if r.URL.Query().Get("song_id") != "42" || r.URL.Query().Get("rating") != "10" {
				t.Error("invalid rating request")
			}
			_, _ = w.Write([]byte(`{"status":"success","song_id":42,"rating":10}`))
		default:
			http.NotFound(w, r)
		}
	}))
	path := filepath.Join(t.TempDir(), "private", "rp-session.json")
	c := New(path)
	c.UserAgent = "omarchy-sonos/test-version"
	c.HTTP.Transport = srv.Client().Transport
	c.Base = srv.URL
	if err := c.Rate(ctx, 42, 10); err == nil {
		t.Fatal("unauthenticated rating accepted")
	}
	if err := c.Login(ctx, "tester", "password"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"password"`) {
		t.Fatal("password persisted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("session is not private")
	}
	restored := New(path)
	restored.UserAgent = c.UserAgent
	restored.HTTP.Transport = srv.Client().Transport
	restored.Base = srv.URL
	if err := restored.Load(); err != nil || !restored.Authenticated() {
		t.Fatal("session did not restore")
	}
	songs, err := restored.Playlist(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 2 || songs[0].ID != 42 || songs[0].ListenerRating != 7.8 || songs[0].UserRating != 9 || songs[1].UserRating != 8 || songs[0].Artist != "Artist & Co" || songs[0].Cover != "https://img.radioparadise.com/covers/l/1.jpg" {
		t.Fatalf("wrong song fields: %+v", songs)
	}
	comments, err := restored.Comments(ctx, 42, 20)
	if err != nil {
		t.Fatal(err)
	}
	if comments.Total != 21 || comments.Items[0].Message != "Hi\nA & B" || comments.Items[0].Upvotes != 3 {
		t.Fatalf("wrong comments: %+v", comments)
	}
	if err = restored.Rate(ctx, 42, 0); err == nil {
		t.Fatal("invalid rating accepted")
	}
	if err = restored.Rate(ctx, 42, 10); err != nil {
		t.Fatal(err)
	}
	if ratings != 1 {
		t.Fatal("invalid request reached server")
	}
	if err = restored.Logout(); err != nil {
		t.Fatal(err)
	}
	if restored.Authenticated() {
		t.Fatal("still authenticated")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("session still exists")
	}
}

func TestMixesIncludeAllSupportedChannels(t *testing.T) {
	mixes := Mixes()
	if len(mixes) != len(Channels)+1 || mixes[0].ID != -1 {
		t.Fatal("missing automatic selection or supported mixes")
	}
	for i, mix := range mixes[1:] {
		if mix.Name != Channels[mix.ID] || (i > 0 && mix.ID <= mixes[i].ID) {
			t.Fatalf("mix list drifted from supported channels: %+v", mixes)
		}
	}
}
func TestErrorsNeverExposePasswordOrToken(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/?password=secret", 302)
	}))
	c := New(filepath.Join(t.TempDir(), "session"))
	c.HTTP.Transport = srv.Client().Transport
	c.Base = srv.URL
	err := c.Login(t.Context(), "username", "extremely-secret")
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "username") {
		t.Fatalf("unsafe auth error: %v", err)
	}
	c.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, &urlError{message: r.URL.String()} })
	err = c.Login(t.Context(), "username", "extremely-secret")
	if err == nil || strings.Contains(err.Error(), "extremely-secret") {
		t.Fatalf("unsafe network error: %v", err)
	}
}

func TestLoginPreservesNumericAccountID(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","username":"tester","passwd":"token","user_id":9007199254740993}`))
	}))
	c := New(filepath.Join(t.TempDir(), "session.json"))
	c.HTTP.Transport = srv.Client().Transport
	if err := c.Login(t.Context(), "tester", "password"); err != nil {
		t.Fatal(err)
	}
	if c.Session.UserID != "9007199254740993" {
		t.Fatalf("account ID lost precision: %s", c.Session.UserID)
	}
	restored := New(c.SessionPath)
	if err := restored.Load(); err != nil || restored.Session != c.Session {
		t.Fatalf("session did not round-trip: %v", err)
	}
}

func TestRPResponseJSONValidation(t *testing.T) {
	for _, tt := range []struct {
		name, body string
	}{
		{"duplicate member", `{"status":"success","status":"failure"}`},
		{"escaped duplicate member", `{"status":"success","\u0073tatus":"failure"}`},
		{"invalid UTF-8", "{\"status\":\"\xff\"}"},
		{"trailing garbage", `{"status":"success"}garbage`},
		{"multiple values", `{"status":"success"}{"status":"failure"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			c := New("")
			c.HTTP.Transport = srv.Client().Transport
			err := c.Login(t.Context(), "tester", "private-password")
			if err == nil || err.Error() != "invalid Radio Paradise response" {
				t.Fatalf("expected sanitized JSON error, got %v", err)
			}
			if c.Authenticated() {
				t.Fatal("invalid response authenticated account")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type urlError struct{ message string }

func (e *urlError) Error() string { return e.message }

func TestPublicPlaylistAndMalformedResponses(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("sent cookies without account")
		}
		_, _ = w.Write([]byte(`{"song":[{"song_id":"1","artist":"A","title":"T","listener_rating":"8.2","rating":null}]}`))
	}))
	c := New("")
	c.HTTP.Transport = srv.Client().Transport
	c.Base = srv.URL
	songs, err := c.Playlist(t.Context(), 0)
	if err != nil || songs[0].UserRating != 0 || songs[0].ListenerRating != 8.2 {
		t.Fatalf("public metadata: %+v %v", songs, err)
	}
	if coverURL("javascript:alert(1)", "") != "" {
		t.Fatal("unsafe cover URL accepted")
	}
}

// Opt-in read-only check: no credentials, rating submissions, or speaker control.
func TestLivePublicRP(t *testing.T) {
	if os.Getenv("RP_LIVE_TEST") != "1" {
		t.Skip("set RP_LIVE_TEST=1 for public RP API smoke test")
	}
	c := New("")
	songs, err := c.Playlist(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) == 0 || songs[0].ID <= 0 || songs[0].Title == "" || songs[0].Cover == "" {
		t.Fatalf("incomplete live song: %+v", songs)
	}
	t.Log("Public RP metadata decoded with cover and listener ratings")
}

func TestCommentsRequireAccount(t *testing.T) {
	c := New("")
	_, err := c.Comments(t.Context(), 42, 0)
	if !errors.Is(err, ErrCommentsAuth) {
		t.Fatalf("expected sign-in prompt, got %v", err)
	}
}
