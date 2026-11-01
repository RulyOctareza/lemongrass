// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliohistory

import (
	"errors"
	"testing"
)

// withoutGit stands in for a machine without git for the rest of the test.
func withoutGit(t *testing.T) {
	t.Helper()
	old := resolveGit
	resolveGit = func() (string, error) { return "", errors.New("osdeps: git is not installed") }
	t.Cleanup(func() { resolveGit = old })
}

func TestMissingGit(t *testing.T) {
	r, _ := newTestRepo(t)
	withoutGit(t)

	if _, err := r.Log("", 5); !errors.Is(err, ErrNoGit) {
		t.Fatalf("log err = %v", err)
	}
	if _, err := r.Snapshot(nil); !errors.Is(err, ErrNoGit) {
		t.Fatalf("snapshot err = %v", err)
	}
}
