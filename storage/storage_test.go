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

func TestLoadMissingAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		name string
		load func(string, any) error
	}{
		{"state", Load},
		{"private", LoadPrivate},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			var got map[string]string
			if err := tt.load(path, &got); err != nil || got != nil {
				t.Fatalf("missing state: %v", err)
			}
			if err := os.WriteFile(path, []byte(`{"token":`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := tt.load(path, &got); err == nil {
				t.Fatal("malformed state accepted")
			}
		})
	}
}

func TestLoadPermissionsRequiredOnlyForPrivateState(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0640, 0644} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(`{"value":"saved"}`), mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			var state map[string]string
			if err := Load(path, &state); err != nil || state["value"] != "saved" {
				t.Fatalf("non-secret state rejected: %v", err)
			}
			var private map[string]string
			err := LoadPrivate(path, &private)
			if mode == 0600 {
				if err != nil || private["value"] != "saved" {
					t.Fatalf("private state rejected: %v", err)
				}
				return
			}
			if err == nil || private != nil {
				t.Fatal("loose private state was not rejected before decoding")
			}
		})
	}
}
