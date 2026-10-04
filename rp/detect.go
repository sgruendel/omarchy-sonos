// Package rp implements Radio Paradise metadata and account APIs, following rptui.
package rp

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

var Channels = map[int]string{0: "Main Mix", 1: "Mellow Mix", 2: "RockIt!", 3: "The Globe", 5: "Beyond", 42: "Serenity", 945: "KFAT"}

// Detect recognizes RP stream hosts or RP station labels; unknown mixes remain unknown.
func Detect(uri, station string) (bool, int) {
	decoded := html.UnescapeString(uri)
	for range 3 {
		v, err := url.QueryUnescape(decoded)
		if err != nil || v == decoded {
			break
		}
		decoded = v
	}
	label := strings.ToLower(station)
	u, _ := url.Parse(strings.TrimPrefix(decoded, "x-rincon-mp3radio://"))
	host := ""
	if u != nil {
		host = strings.ToLower(u.Hostname())
	}
	if host == "" {
		u, _ = url.Parse("https://" + strings.TrimPrefix(decoded, "x-rincon-mp3radio://"))
		if u != nil {
			host = strings.ToLower(u.Hostname())
		}
	}
	isRP := host == "radioparadise.com" || strings.HasSuffix(host, ".radioparadise.com") || host == "radioparadise.stream" || strings.HasSuffix(host, ".radioparadise.stream") || strings.Contains(label, "radio paradise") || strings.Contains(label, "radioparadise")
	if !isRP {
		return false, -1
	}
	text := label + " " + strings.ToLower(decoded)
	for _, pair := range []struct {
		id    int
		names []string
	}{{42, []string{"serenity"}}, {945, []string{"kfat"}}, {5, []string{"beyond"}}, {1, []string{"mellow"}}, {2, []string{"rockit", "rock mix", "rock-"}}, {3, []string{"globe", "global", "world", "eclectic"}}, {0, []string{"main mix", "main-mix", "main-"}}} {
		for _, name := range pair.names {
			if strings.Contains(text, name) {
				return true, pair.id
			}
		}
	}
	if u != nil {
		// Native Sonos RP playback uses /audio/blocks/<channel>/... on the .stream CDN.
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "audio" && parts[1] == "blocks" {
			id, err := strconv.Atoi(parts[2])
			if _, ok := Channels[id]; err == nil && ok {
				return true, id
			}
		}
		if q := u.Query().Get("chan"); q != "" {
			id, err := strconv.Atoi(q)
			if _, ok := Channels[id]; err == nil && ok {
				return true, id
			}
		}
		path := strings.ToLower(u.Path)
		if strings.HasPrefix(path, "/aac-") || strings.HasPrefix(path, "/mp3-") || strings.HasPrefix(path, "/flac") {
			return true, 0
		}
	}
	return true, -1
}
func normalized(s string) string {
	s = html.UnescapeString(strings.ToLower(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, s)
}

// Matches requires artist + title, including Sonos's unparsed streamContent form.
func Matches(song Song, title, artist, streamContent string) bool {
	if normalized(song.Title) == "" || normalized(song.Artist) == "" {
		return false
	}
	if normalized(title) == normalized(song.Title) && normalized(artist) == normalized(song.Artist) {
		return true
	}
	combined := normalized(song.Artist + song.Title)
	return normalized(streamContent) == combined || (artist == "" && normalized(title) == combined)
}
