// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package guard

import (
	"path/filepath"
	"syscall"
	"testing"
)

// Windows has no named pipes in the filesystem, so the fifo case only runs on Unix.
func TestScriptFileFifo(t *testing.T) {
	root, home, _ := rmProject(t)
	if err := syscall.Mkfifo(filepath.Join(root, "fifo.py"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ruleOf(decideIn(t, root, home, root, "python3 fifo.py", Policy{})); got != "" {
		t.Errorf("rule %q, want none", got)
	}
}
