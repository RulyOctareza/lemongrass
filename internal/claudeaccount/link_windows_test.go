// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package claudeaccount

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJunctionLinksAndRemovesWithoutFollowing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "primary", "projects")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "memory.md"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "account", "projects")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := symlinkOnce(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := symlinkOnce(src, dst); err != nil {
		t.Fatalf("a matching junction made earlier is not success: %v", err)
	}
	info, err := os.Lstat(dst)
	if err != nil || !isLink(info) {
		t.Fatalf("junction not seen as a link: %v %v", info, err)
	}
	target, err := os.Readlink(dst)
	if err != nil || !sameLinkTarget(target, src) {
		t.Fatalf("Readlink = %q, %v; want %q", target, err, src)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "memory.md")); err != nil || string(data) != "kept" {
		t.Fatalf("reading through the junction: %q %v", data, err)
	}
	if err := removeIfSymlink(dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("junction still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "memory.md")); err != nil {
		t.Fatalf("removing the junction reached the primary: %v", err)
	}
}

func TestSameLinkTargetForms(t *testing.T) {
	if !sameLinkTarget(`\??\C:\Users\a\.claude\projects`, `c:\users\a\.claude\projects\`) {
		t.Error("NT prefix, case and trailing separator should compare equal")
	}
	if sameLinkTarget(`C:\Users\a\.claude\skills`, `C:\Users\a\.claude\projects`) {
		t.Error("different folders compare equal")
	}
}
