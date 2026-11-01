// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package osdeps checks the tools lemongrass needs from the OS: where each one is, its version, which package owns
// it, and what install or upgrade the app can offer for it on this OS.
package osdeps

// Dep is one tool lemongrass runs but does not ship.
type Dep struct {
	Name     string
	Required bool
	// Fixed camelCase ids the app maps to its own copy.
	Features []string
	// The lowest version lemongrass works with, empty when any version does.
	Floor string
	// The arguments that make the tool print its version.
	VersionArgs []string
	// A tool lemongrass only reports and never installs, such as ssh.
	CheckOnly bool
	// The package a fresh install uses, per OS family. A family missing here has no route.
	Packages map[string]string
	// Said when the tool is missing and its family has no package.
	NoRoute map[string]string
}

// Fixed so the cache can tell when the list itself changed.
var Deps = []Dep{
	{
		Name:     "git",
		Required: true,
		Features: []string{"gitPane", "conflicts", "rebaseForecast", "biblioHistory", "workspaces"},
		// merge-tree --merge-base takes a tree only from 2.45.0, which the rebase forecast and workspaces need.
		Floor:       "2.45.0",
		VersionArgs: []string{"--version"},
		Packages:    map[string]string{FamilyFedora: "git"},
	},
	{
		Name:        "adb",
		Features:    []string{"device", "flutterRecording"},
		VersionArgs: []string{"version"},
		Packages:    map[string]string{FamilyFedora: "android-tools"},
	},
	{
		Name:        "scrcpy",
		Features:    []string{"device", "flutterRecording"},
		VersionArgs: []string{"--version"},
		NoRoute: map[string]string{
			FamilyFedora: "scrcpy has no package in the repositories this system has enabled. Its developers publish it for Fedora through the COPR repository zeno/scrcpy.",
		},
	},
	{
		Name:        "ffmpeg",
		Features:    []string{"flutterRecording"},
		VersionArgs: []string{"-version"},
		// RPM Fusion's full ffmpeg conflicts with ffmpeg-free, so a fresh install takes ffmpeg-free and an upgrade
		// names whichever package owns the binary.
		Packages: map[string]string{FamilyFedora: "ffmpeg-free"},
	},
	{
		Name:        "ssh",
		Features:    []string{"sshChannels"},
		VersionArgs: []string{"-V"},
		CheckOnly:   true,
		NoRoute: map[string]string{
			FamilyFedora: "ssh is not installed. Database channels over SSH need it, from the openssh-clients package.",
		},
	},
}

// Find returns the dependency by name.
func Find(name string) (Dep, bool) {
	for _, d := range Deps {
		if d.Name == name {
			return d, true
		}
	}
	return Dep{}, false
}
