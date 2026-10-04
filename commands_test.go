package main

import (
	"encoding/json/v2"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
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
			if err := a.Execute(t.Context(), Command{Op: "adjustVolume", Delta: new(tt.delta)}); err != nil || writes != 1 {
				t.Fatalf("volume command: %v, writes=%d", err, writes)
			}
		})
	}
}
