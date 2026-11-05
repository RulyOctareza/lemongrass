// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"path/filepath"
)

const macGitStub = "/usr/bin/git"

// candidates lists where a tool is installed besides PATH. An app started from a desktop launcher, macOS Finder above
// all, gets a minimal PATH, and an agent's shell may not have Android platform-tools on its PATH.
func candidates(sys System, name string) []string {
	if sys.GOOS == "windows" {
		return windowsCandidates(sys, name)
	}
	var dirs []string
	if name == "adb" {
		for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
			if dir := sys.Getenv(env); dir != "" {
				dirs = append(dirs, filepath.Join(dir, "platform-tools"))
			}
		}
		dirs = append(dirs,
			filepath.Join(sys.Home, "Library", "Android", "sdk", "platform-tools"),
			filepath.Join(sys.Home, "Android", "Sdk", "platform-tools"),
		)
	}
	if sys.GOOS == "darwin" {
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/usr/bin")
	} else {
		dirs = append(dirs, "/usr/bin", "/usr/local/bin", "/opt/homebrew/bin")
	}
	paths := make([]string, len(dirs))
	for i, dir := range dirs {
		paths[i] = filepath.Join(dir, name)
	}
	return paths
}

// Resolve finds a tool's path, PATH first, or returns "" when it is not installed. On macOS /usr/bin/git is a stub
// that opens the Command Line Tools installer dialog when run, which a background check must never do, so it counts
// only once those tools are installed.
func Resolve(ctx context.Context, sys System, name string) string {
	usable := func(p string) bool {
		if sys.GOOS == "darwin" && p == macGitStub {
			_, err := sys.Run(ctx, "xcode-select", "-p")
			return err == nil
		}
		return true
	}
	onPath, err := sys.LookPath(name)
	if err == nil && usable(onPath) && !bundledAdb(sys, name, onPath) {
		return onPath
	}
	for _, c := range candidates(sys, name) {
		if st, ok := sys.Stat(c); ok && !st.Dir && st.Exec && usable(c) && !bundledAdb(sys, name, c) {
			return c
		}
	}
	// scrcpy's own adb is still better than none.
	if err == nil && usable(onPath) {
		return onPath
	}
	return ""
}
