// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterCreatesEntryMatchingElectronUIShape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	p, err := Register(dir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.ID == "" {
		t.Error("Register did not assign an id")
	}
	if p.Name != filepath.Base(dir) {
		t.Errorf("Name = %q, want %q (basename)", p.Name, filepath.Base(dir))
	}
	if p.Path != dir {
		t.Errorf("Path = %q, want %q", p.Path, dir)
	}

	resolved, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve after Register: %v", err)
	}
	if resolved.ID != p.ID {
		t.Errorf("Resolve returned a different project than Register created: %+v vs %+v", resolved, p)
	}
}

func TestRegisterIsIdempotentByExactPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	first, err := Register(dir)
	if err != nil {
		t.Fatalf("Register (first): %v", err)
	}
	second, err := Register(dir)
	if err != nil {
		t.Fatalf("Register (second): %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("Register on an already-registered path returned a new id: %q vs %q", first.ID, second.ID)
	}

	data, err := os.ReadFile(registryPath())
	if err != nil {
		t.Fatalf("reading registry: %v", err)
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		t.Fatalf("parsing registry: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("registry has %d entries after registering the same path twice, want 1", len(projects))
	}
}

func TestResolveFollowsSymlinkToRegisteredPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	real := t.TempDir()
	p, err := Register(real)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	resolved, err := Resolve(link)
	if err != nil {
		t.Fatalf("Resolve(symlink): %v", err)
	}
	if resolved.ID != p.ID {
		t.Errorf("Resolve(symlink) = %+v, want project %+v", resolved, p)
	}

	nestedReal := filepath.Join(real, "sub", "dir")
	if err := os.MkdirAll(nestedReal, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	nestedViaLink := filepath.Join(link, "sub", "dir")
	resolved, err = Resolve(nestedViaLink)
	if err != nil {
		t.Fatalf("Resolve(nested under symlink): %v", err)
	}
	if resolved.ID != p.ID {
		t.Errorf("Resolve(nested under symlink) = %+v, want project %+v", resolved, p)
	}
}

func TestRegisterViaSymlinkReturnsExistingEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	real := t.TempDir()
	first, err := Register(real)
	if err != nil {
		t.Fatalf("Register(real): %v", err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	second, err := Register(link)
	if err != nil {
		t.Fatalf("Register(symlink): %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("Register(symlink) created a new entry %+v instead of returning the existing one %+v", second, first)
	}

	data, err := os.ReadFile(registryPath())
	if err != nil {
		t.Fatalf("reading registry: %v", err)
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		t.Fatalf("parsing registry: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("registry has %d entries after registering the real path then its symlink, want 1", len(projects))
	}
}
