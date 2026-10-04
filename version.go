package main

import (
	_ "embed"
	"encoding/json/v2"
	"errors"
	"fmt"
)

// Use the shipped manifest for both development builds and release bundles.
//
//go:embed manifest.json
var pluginManifest []byte

func manifestVersion() (string, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(pluginManifest, &manifest); err != nil {
		return "", fmt.Errorf("decode embedded plugin manifest: %w", err)
	}
	if manifest.Version == "" {
		return "", errors.New("embedded plugin manifest has no version")
	}
	return manifest.Version, nil
}
