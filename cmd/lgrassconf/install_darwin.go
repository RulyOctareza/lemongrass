// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build darwin

package main

import (
	"fmt"
	"github.com/faizalv/lemongrass/internal/agentconf"
	"os"
	"os/exec"
	"path/filepath"
)

func installService(home string) error {
	binary, err := resolveBinary()
	if err != nil {
		return err
	}

	logDir := filepath.Join(home, "Library", "Logs", "lemongrass")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchAgentLabel, binary,
		filepath.Join(logDir, "lgrassconf.out.log"),
		filepath.Join(logDir, "lgrassconf.err.log"))

	agentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}
	plistPath := filepath.Join(agentsDir, launchAgentLabel+".plist")
	if err := agentconf.WriteAtomic(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}

	uid := fmt.Sprintf("gui/%d", os.Getuid())
	legacyPath := filepath.Join(agentsDir, legacyLaunchLabel+".plist")
	_ = exec.Command("launchctl", "bootout", uid, legacyPath).Run()
	_ = os.Remove(legacyPath)
	_ = exec.Command("launchctl", "bootout", uid, plistPath).Run()

	cmd := exec.Command("launchctl", "bootstrap", uid, plistPath)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("launchctl", "load", "-w", plistPath)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err2 := cmd.Run(); err2 != nil {
			return fmt.Errorf("launchctl bootstrap/load: %v / %w", err, err2)
		}
	}
	return nil
}

func uninstallService(home string) error {
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", uid, plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
