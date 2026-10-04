package storage

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRoundTripAndRepairPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "session.json")
	want := map[string]string{"token": "private-value"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(path), 0700}, {path, 0600}} {
		info, err := os.Stat(tt.path)
		if err != nil || info.Mode().Perm() != tt.mode {
			t.Fatalf("private mode for %s: %v, %v", tt.path, info, err)
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	want["token"] = "replacement-value"
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("replacement did not repair permissions: %v", err)
	}
	var got map[string]string
	if err := Load(path, &got); err != nil || got["token"] != want["token"] {
		t.Fatalf("state did not round-trip: %v", err)
	}
}

func TestFailedSavePreservesExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, map[string]string{"value": "old"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, math.Inf(1)); err == nil {
		t.Fatal("unsupported JSON value saved")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed save damaged existing state")
	}
	// A failed rename must also clean up the temporary private file.
	blocked := filepath.Join(filepath.Dir(path), "directory")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Save(blocked, "cannot replace a directory"); err == nil {
		t.Fatal("replaced a directory with state")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 2 || entries[0].Name() != "directory" || entries[1].Name() != "state.json" {
		t.Fatalf("failed save left temporary files: %v, %v", entries, err)
	}
}

func TestLoadMissingMalformedAndLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	var got map[string]string
	if err := Load(filepath.Join(dir, "missing.json"), &got); err != nil || got != nil {
		t.Fatalf("missing state: %v", err)
	}
	for _, tt := range []struct {
		name, data string
		mode       os.FileMode
	}{
		{"malformed", `{"token":`, 0600},
		{"public", `{"token":"private"}`, 0644},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".json")
			if err := os.WriteFile(path, []byte(tt.data), tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := Load(path, &got); err == nil {
				t.Fatal("invalid private state accepted")
			}
		})
	}
}
