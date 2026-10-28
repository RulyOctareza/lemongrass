// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustCodexProjectAppendsATable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("model = \"m\"\n[tui]\nx = true"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := TrustCodexProject(path, "/p/app"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "model = \"m\"\n[tui]\nx = true\n\n[projects.\"/p/app\"]\ntrust_level = \"trusted\"\n"
	if string(data) != want {
		t.Errorf("config =\n%s", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func TestTrustCodexProjectLeavesAnExistingChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "[projects.\"/p/app\"]\ntrust_level = \"untrusted\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := TrustCodexProject(path, "/p/app"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != original {
		t.Errorf("an existing choice was changed:\n%s", data)
	}
}

func TestTrustCodexProjectQuotesThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := TrustCodexProject(path, `/p/a"b\c`); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `[projects."/p/a\"b\\c"]`) {
		t.Errorf("path not escaped:\n%s", data)
	}
}

func TestTrustCodexProjectRefusesInlineProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("projects = { }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := TrustCodexProject(path, "/p/app"); err == nil {
		t.Error("an inline projects table must be refused")
	}
}

func TestTrustCodexProjectSkipsWithoutCodex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.toml")
	if err := TrustCodexProject(path, "/p/app"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("no file may be created without a Codex directory: %v", err)
	}
}
