// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

func TestTrustCommandTargets(t *testing.T) {
	home := t.TempDir()
	accounts := claudeaccount.Accounts{Home: home}
	if _, err := accounts.Add("work"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "work"} {
		if err := os.WriteFile(accounts.ClaudeJSONPath(name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	codex := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codex), 0o700); err != nil {
		t.Fatal(err)
	}
	// A rooted path without a drive is not absolute on Windows, which trust refuses.
	abs := func(p string) string {
		if runtime.GOOS == "windows" {
			return filepath.FromSlash("C:" + p)
		}
		return p
	}
	// Both .claude.json and config.toml escape a backslash in the path as two.
	has := func(path, project string) bool {
		data, _ := os.ReadFile(path)
		return strings.Contains(string(data), strings.ReplaceAll(project, `\`, `\\`))
	}

	if err := runTrust(accounts, codex, []string{abs("/p/one"), "--target", "work"}); err != nil {
		t.Fatal(err)
	}
	if !has(accounts.ClaudeJSONPath("work"), abs("/p/one")) || has(accounts.ClaudeJSONPath(""), abs("/p/one")) || has(codex, abs("/p/one")) {
		t.Error("a single target must write only that target")
	}
	if err := runTrust(accounts, codex, []string{abs("/p/two")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{accounts.ClaudeJSONPath(""), accounts.ClaudeJSONPath("work"), codex} {
		if !has(path, abs("/p/two")) {
			t.Errorf("%s not trusted by the all-target call", path)
		}
	}
	if err := runTrust(accounts, codex, []string{abs("/p/one"), "--target", "ghost"}); err == nil {
		t.Error("an unknown account must be refused")
	}
	for _, args := range [][]string{{"relative"}, {}, {"/p", "--target"}} {
		if err := runTrust(accounts, codex, args); !errors.Is(err, errTrustUsage) {
			t.Errorf("%v: want usage error, got %v", args, err)
		}
	}
}
