package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"slices"
	"time"
)

type Command struct {
	ID       string `json:"id"`
	Op       string `json:"op"`
	Room     string `json:"room"`
	Volume   *int   `json:"volume"`
	Delta    *int   `json:"delta"`
	Mute     *bool  `json:"mute"`
	Channel  *int   `json:"channel"`
	SongID   *int64 `json:"songId"`
	Rating   *int   `json:"rating"`
	Offset   *int   `json:"offset"`
	Username string `json:"username"`
	Password string `json:"password"`
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
			line := slices.Clone(scanner.Bytes())
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
