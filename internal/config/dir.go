// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package config holds path helpers for lemongrass's own runtime directory.
package config

import (
	"os"
	"path/filepath"
)

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".lemongrass")
}
