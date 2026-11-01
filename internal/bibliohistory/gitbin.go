// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliohistory

import (
	"errors"

	"github.com/faizalv/lemongrass/internal/osdeps"
)

// ErrNoGit means no usable git was found, so history is not recorded or read.
var ErrNoGit = errors.New("bibliohistory: git is not installed, so biblio history is off")

// Replaced in tests to stand in for a machine without git.
var resolveGit = func() (string, error) { return osdeps.Path("git") }

// GitPath finds git where the dependency check finds it, which also covers the minimal PATH of an app started from a
// desktop launcher and never runs the macOS git stub that opens an installer dialog.
func GitPath() (string, error) {
	p, err := resolveGit()
	if err != nil {
		return "", ErrNoGit
	}
	return p, nil
}
