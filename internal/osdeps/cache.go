// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/faizalv/lemongrass/internal/config"
)

// Raised whenever the cache's layout or the meaning of a field changes, so an older file is thrown away.
const cacheSchema = 1

type cacheEntry struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Version string `json:"version"`
	Origin  Origin `json:"origin"`
}

type cacheFile struct {
	Schema  int                   `json:"schema"`
	OS      OSInfo                `json:"os"`
	Atomic  bool                  `json:"atomic"`
	Winget  bool                  `json:"winget"`
	List    string                `json:"list"`
	Entries map[string]cacheEntry `json:"entries"`
}

// CachePath is where the check keeps what it learned between runs.
func CachePath() string {
	return filepath.Join(config.Dir(), "deps.json")
}

// listKey changes whenever a dependency is added or removed or its floor moves.
func listKey() string {
	parts := make([]string, len(Deps))
	for i, d := range Deps {
		parts[i] = d.Name + "@" + d.Floor
	}
	return strings.Join(parts, ",")
}

// loadCache returns the cache's entries when they still describe this OS and this dependency list, or nil.
func loadCache(path string, info OSInfo) map[string]cacheEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c cacheFile
	if json.Unmarshal(data, &c) != nil {
		return nil
	}
	// The OS block is compared field by field, since the fields kept out of JSON never survive the file.
	same := c.OS.Family == info.Family && c.OS.ID == info.ID && c.OS.Version == info.Version &&
		c.Atomic == info.Atomic && c.Winget == info.Winget
	if c.Schema != cacheSchema || !same || c.List != listKey() {
		return nil
	}
	return c.Entries
}

// saveCache writes through a temp file and a rename, so a check killed midway never leaves half a file.
func saveCache(path string, info OSInfo, entries map[string]cacheEntry) error {
	c := cacheFile{Schema: cacheSchema, OS: info, Atomic: info.Atomic, Winget: info.Winget, List: listKey(), Entries: entries}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("osdeps: encode cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("osdeps: create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".deps-*.json")
	if err != nil {
		return fmt.Errorf("osdeps: write cache: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("osdeps: write cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("osdeps: write cache: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("osdeps: write cache: %w", err)
	}
	return nil
}
