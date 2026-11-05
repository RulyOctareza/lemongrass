// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package config

const exeSuffix = ""

// The app falls back to /usr/local/bin when ~/.local/bin does not exist.
var fallbackBinDirs = []string{"/usr/local/bin"}
