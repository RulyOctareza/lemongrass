// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func memoryGateFixture(t *testing.T) (home, projectPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	projectPath = t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, projectPath
}
