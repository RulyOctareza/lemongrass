// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !linux && !darwin

package main

import "fmt"

func installService(home string) error {
	_ = home
	return fmt.Errorf("lgrassconf install is only supported on Linux (systemd) and macOS (LaunchAgent)")
}

func uninstallService(home string) error {
	_ = home
	return fmt.Errorf("lgrassconf uninstall is only supported on Linux (systemd) and macOS (LaunchAgent)")
}
