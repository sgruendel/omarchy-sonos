// Package storage persists JSON state with private, atomic replacement.
package storage

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
)

// Save atomically replaces private state. Credentials never appear in snapshots.
func Save(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// Load decodes JSON state. Missing files leave value unchanged.
func Load(path string, value any) error {
	return load(path, value, false)
}

// LoadPrivate also rejects files with group or other permissions before decoding.
func LoadPrivate(path string, value any) error {
	return load(path, value, true)
}

func load(path string, value any, private bool) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if private {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 {
			return errors.New("private state file has group or other permissions")
		}
	}
	return json.UnmarshalRead(f, value)
}
