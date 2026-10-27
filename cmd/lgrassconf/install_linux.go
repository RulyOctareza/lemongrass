// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

//go:build linux

package main

import (
	"fmt"
	"github.com/faizalv/lemongrass/internal/agentconf"
	"os"
	"os/exec"
	"path/filepath"
)

func unitText(binary string) string {
	return fmt.Sprintf(`[Unit]
Description=lemongrass agent config keeper

[Service]
Type=simple
ExecStart=%s run
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, binary)
}

func installService(home string) error {
	binary, err := resolveBinary()
	if err != nil {
		return err
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", unitName)
	if err := agentconf.WriteAtomic(unitPath, []byte(unitText(binary)), 0o644); err != nil {
		return err
	}
	legacy := exec.Command("systemctl", "--user", "disable", "--now", legacyUnitName)
	legacy.Stdout, legacy.Stderr = os.Stdout, os.Stderr
	if err := legacy.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return fmt.Errorf("systemctl --user disable %s: %w", legacyUnitName, err)
		}
	}
	legacyUnitPath := filepath.Join(home, ".config", "systemd", "user", legacyUnitName)
	if err := os.Remove(legacyUnitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", unitName},
		{"restart", unitName},
	} {
		cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("systemctl --user %s: %w", args[0], err)
		}
	}
	return nil
}

func uninstallService(home string) error {
	disable := exec.Command("systemctl", "--user", "disable", "--now", unitName)
	disable.Stdout, disable.Stderr = os.Stdout, os.Stderr
	if err := disable.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return fmt.Errorf("systemctl --user disable %s: %w", unitName, err)
		}
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", unitName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	reload := exec.Command("systemctl", "--user", "daemon-reload")
	reload.Stdout, reload.Stderr = os.Stdout, os.Stderr
	if err := reload.Run(); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w", err)
	}
	return nil
}
