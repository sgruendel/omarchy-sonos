package rp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"omarchy-sonos/internal/storage"
)

type Session struct {
	Username      string `json:"username"`
	PasswordToken string `json:"passwordToken"`
	UserID        string `json:"userId"`
}
type Song struct {
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	Artist         string  `json:"artist"`
	Album          string  `json:"album"`
	Year           string  `json:"year"`
	Cover          string  `json:"cover"`
	ListenerRating float64 `json:"listenerRating"`
	RatingsCount   int     `json:"ratingsCount"`
	UserRating     int     `json:"userRating"`
}
type Comment struct {
	Username  string `json:"username"`
	Posted    string `json:"posted"`
	Message   string `json:"message"`
	Upvotes   int    `json:"upvotes"`
	Downvotes int    `json:"downvotes"`
}
type Comments struct {
	Items  []Comment `json:"items"`
	Total  int       `json:"total"`
	More   bool      `json:"more"`
	Offset int       `json:"offset"`
}
type Client struct {
	HTTP        *http.Client
	Base        string
	Session     Session
	SessionPath string
}

var ErrCommentsAuth = errors.New("Sign in to Radio Paradise to read song comments")

func New(path string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, Base: "https://api.radioparadise.com", SessionPath: path}
}
func (c *Client) Authenticated() bool { return c.Session.PasswordToken != "" && c.Session.UserID != "" }
func (c *Client) Load() error         { return storage.Load(c.SessionPath, &c.Session) }
func (c *Client) request(ctx context.Context, path string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.Base+path+"?"+params.Encode(), nil)
	if err != nil {
		return errors.New("could not create RP request")
	}
	req.Header.Set("User-Agent", "omarchy-sonos/0.1")
	if c.Authenticated() {
		for _, cookie := range []*http.Cookie{{Name: "player_id", Value: "omarchy-sonos"}, {Name: "C_username", Value: c.Session.Username}, {Name: "C_passwd", Value: c.Session.PasswordToken}, {Name: "C_validated", Value: "yes"}, {Name: "C_user_id", Value: c.Session.UserID}} {
			req.AddCookie(cookie)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("Radio Paradise request failed (network or timeout)")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusUnauthorized && c.Authenticated() {
			return errors.New("Radio Paradise session expired; sign in again")
		}
		return fmt.Errorf("Radio Paradise returned HTTP %d", resp.StatusCode)
	}
	d := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	d.UseNumber()
	if err = d.Decode(out); err != nil {
		return errors.New("invalid Radio Paradise response")
	}
	return nil
}
func (c *Client) Login(ctx context.Context, username, password string) error {
	if strings.TrimSpace(username) == "" || password == "" {
		return errors.New("enter your RP username and password")
	}
	var result map[string]any
	// RP's existing auth API uses query parameters. Never include request URLs in errors.
	if err := c.request(ctx, "/api/auth", url.Values{"username": {username}, "passwd": {password}}, &result); err != nil {
		return err
	}
	if str(result["status"]) != "success" || str(result["passwd"]) == "" || str(result["user_id"]) == "" {
		return errors.New("Radio Paradise login failed; check your credentials")
	}
	session := Session{Username: str(result["username"]), PasswordToken: str(result["passwd"]), UserID: str(result["user_id"])}
	if session.Username == "" {
		session.Username = username
	}
	if err := storage.Save(c.SessionPath, session); err != nil {
		return errors.New("could not save RP session")
	}
	c.Session = session
	return nil
}
func (c *Client) Logout() error {
	if err := os.Remove(c.SessionPath); err != nil && !os.IsNotExist(err) {
		return errors.New("could not remove RP session")
	}
	c.Session = Session{}
	return nil
}
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func num(v any) float64 { n, _ := strconv.ParseFloat(str(v), 64); return n }
func first(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}
func coverURL(cover, base string) string {
	if cover == "" {
		return ""
	}
	if strings.HasPrefix(cover, "//") {
		cover = "https:" + cover
	}
	if base == "" {
		base = "https://img.radioparadise.com/"
	}
	if strings.HasPrefix(base, "//") {
		base = "https:" + base
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u, err := url.Parse(cover)
	if err != nil {
		return ""
	}
	u = b.ResolveReference(u)
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.String()
}
func (c *Client) Playlist(ctx context.Context, channel int) ([]Song, error) {
	if _, ok := Channels[channel]; !ok {
		return nil, errors.New("select the Radio Paradise mix")
	}
	var result struct {
		Songs     []map[string]any `json:"song"`
		ImageBase string           `json:"image_base"`
	}
	if err := c.request(ctx, "/api/nowplaying_list_v2022", url.Values{"chan": {strconv.Itoa(channel)}}, &result); err != nil {
		return nil, err
	}
	songs := make([]Song, 0, len(result.Songs))
	for _, raw := range result.Songs {
		rating := 0
		if c.Authenticated() {
			rating = int(num(raw["user_rating"]))
			if raw["user_rating"] == nil {
				rating = int(num(raw["rating"]))
			}
		}
		songs = append(songs, Song{ID: int64(num(raw["song_id"])), Title: html.UnescapeString(str(raw["title"])), Artist: html.UnescapeString(str(raw["artist"])), Album: html.UnescapeString(str(raw["album"])), Year: str(raw["year"]), Cover: coverURL(first(raw, "cover_large", "cover", "cover_med"), result.ImageBase), ListenerRating: num(raw["listener_rating"]), RatingsCount: int(num(raw["ratings_num"])), UserRating: rating})
	}
	return songs, nil
}
func Match(songs []Song, title, artist, content string) *Song {
	for _, s := range songs {
		if s.ID > 0 && Matches(s, title, artist, content) {
			return &s
		}
	}
	return nil
}
func (c *Client) Rate(ctx context.Context, songID int64, rating int) error {
	if !c.Authenticated() {
		return errors.New("sign in to Radio Paradise to rate songs")
	}
	if songID <= 0 || rating < 1 || rating > 10 {
		return errors.New("rating must be 1–10 and song must be identified")
	}
	var result map[string]any
	if err := c.request(ctx, "/api/rating", url.Values{"song_id": {strconv.FormatInt(songID, 10)}, "rating": {strconv.Itoa(rating)}}, &result); err != nil {
		return err
	}
	if str(result["status"]) != "success" {
		return errors.New("Radio Paradise rejected the rating; try signing in again")
	}
	return nil
}

var tags = regexp.MustCompile(`<[^>]*>`)
var breaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)
var scripts = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)>`)

func PlainText(s string) string {
	s = scripts.ReplaceAllString(s, "")
	s = breaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}
func (c *Client) Comments(ctx context.Context, songID int64, offset int) (Comments, error) {
	page := Comments{Items: []Comment{}}
	if !c.Authenticated() {
		return page, ErrCommentsAuth
	}
	if songID <= 0 || offset < 0 {
		return page, errors.New("invalid song or comment offset")
	}
	var raw struct {
		Items  []map[string]any `json:"comments"`
		Total  any              `json:"total_comments"`
		More   bool             `json:"more_comments"`
		Offset any              `json:"more_offset"`
	}
	if err := c.request(ctx, "/siteapi.php", url.Values{"file": {"comments::list"}, "song_id": {strconv.FormatInt(songID, 10)}, "comments_num": {"20"}, "order": {"newest"}, "comments_offset": {strconv.Itoa(offset)}}, &raw); err != nil {
		return page, err
	}
	page.Total = int(num(raw.Total))
	page.More = raw.More
	page.Offset = int(num(raw.Offset))
	for _, r := range raw.Items {
		page.Items = append(page.Items, Comment{Username: str(r["username"]), Posted: PlainText(str(r["posted_time"])), Message: PlainText(str(r["message"])), Upvotes: int(num(r["upvotes"])), Downvotes: int(num(r["downvotes"]))})
	}
	if page.More && page.Offset <= offset {
		page.Offset = offset + len(page.Items)
	}
	return page, nil
}
