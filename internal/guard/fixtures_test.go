// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func bashInput(t *testing.T, command string) Input {
	t.Helper()
	return Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: t.TempDir()}
}

// resolvedTempDir is t.TempDir() resolved through symlinks, since the guard reports resolved paths and the temp dir is a symlink on macOS (/tmp -> /private/tmp).
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func rmProject(t *testing.T) (root, home, outside string) {
	t.Helper()
	home = resolvedTempDir(t)
	root = filepath.Join(home, "work", "app")
	outside = resolvedTempDir(t)
	for _, dir := range []string{"build/out", "sub", ".git/refs", "biblio/books"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"a.log", "b.log", "notes.txt", "sub/c.txt", ".git/index.lock", "biblio/books/toc.md"} {
		if err := os.WriteFile(filepath.Join(root, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	return root, home, outside
}

func ruleOf(verdict *Verdict) string {
	if verdict == nil {
		return ""
	}
	return verdict.RuleID
}
