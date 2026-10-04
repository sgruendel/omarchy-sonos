package main

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"omarchy-sonos/storage"
)

func TestRestoredSpeakerStatePreservesPreferencesAndRepairsPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0640, 0644} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			want := State{Selected: "RINCON_A", Hosts: []string{"192.168.1.3"}, Channels: map[string]int{"RINCON_A": 2}}
			if err := storage.Save(path, want); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			a, err := newApp(dir, nil)
			if err != nil {
				t.Fatalf("restored non-secret state prevented startup: %v", err)
			}
			if a.State.Selected != want.Selected || !slices.Equal(a.State.Hosts, want.Hosts) || a.State.Channels["RINCON_A"] != 2 || a.startupWarning != "" {
				t.Fatalf("restored preferences were lost: %+v, warning %q", a.State, a.startupWarning)
			}
			if err := a.save(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("save did not repair restored state permissions: %v, %v", info, err)
			}
		})
	}
}

func TestCorruptSessionDoesNotPreventStartup(t *testing.T) {
	for _, tt := range []struct {
		name, data string
		mode       os.FileMode
	}{
		{"syntax", `{"passwordToken":"private-token","userId":"123",`, 0600},
		// Credentials can decode successfully before a later field fails.
		{"type", `{"passwordToken":"private-token","userId":"123","username":true}`, 0600},
		{"permissions", `{"passwordToken":"private-token","userId":"123","username":"tester"}`, 0644},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rp-session.json")
			if err := os.WriteFile(path, []byte(tt.data), tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tt.mode); err != nil {
				t.Fatal(err)
			}
			a, err := newApp(dir, nil)
			if err != nil {
				t.Fatalf("corrupt session prevented startup: %v", err)
			}
			if a.RP.Authenticated() || a.RP.Session.Username != "" || a.RP.Session.PasswordToken != "" || a.startupWarning == "" {
				t.Fatal("corrupt session did not leave a clean signed-out state")
			}
			encoded, err := json.Marshal(a.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(a.startupWarning+string(encoded), "private-token") {
				t.Fatal("corrupt session leaked credentials into diagnostics")
			}
		})
	}
}

func TestVersionAndMixesComeFromBackendMetadata(t *testing.T) {
	a, err := newApp(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	version, err := manifestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot.BackendVersion != version || a.RP.UserAgent != "omarchy-sonos/"+version {
		t.Fatal("backend version drifted from the plugin manifest")
	}
	if len(a.Snapshot.RPMixes) < 2 || a.Snapshot.RPMixes[0].ID != -1 {
		t.Fatal("snapshot missing supported RP mixes")
	}
}

func TestParseHosts(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  []string
	}{
		{"", nil},
		{" , , ", nil},
		{" 192.168.1.3 , 192.168.1.4 ,, ", []string{"192.168.1.3", "192.168.1.4"}},
	} {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseHosts(tt.input); !slices.Equal(got, tt.want) {
				t.Fatalf("parseHosts(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
