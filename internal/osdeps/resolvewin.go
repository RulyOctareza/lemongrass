// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import "strings"

// Folder names winget gives a package it unpacks, under its Packages folder.
const wingetSource = "_Microsoft.Winget.Source_8wekyb3d8bbwe"

// windowsCandidates are the places Windows installs put a tool without it always being on PATH: winget's links,
// Scoop's shims, Android Studio's SDK, the Git for Windows folders, the OpenSSH capability and winget itself.
func windowsCandidates(sys System, name string) []string {
	local := sys.Getenv("LOCALAPPDATA")
	var dirs []string
	add := func(base string, elem ...string) {
		if base != "" {
			dirs = append(dirs, winJoin(append([]string{base}, elem...)...))
		}
	}
	switch name {
	case "adb":
		add(sys.Getenv("ANDROID_HOME"), "platform-tools")
		add(sys.Getenv("ANDROID_SDK_ROOT"), "platform-tools")
		add(local, "Android", "Sdk", "platform-tools")
		add(local, "Microsoft", "WinGet", "Packages", "Google.PlatformTools"+wingetSource, "platform-tools")
	case "git":
		for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
			add(sys.Getenv(env), "Git", "cmd")
		}
		add(local, "Programs", "Git", "cmd")
	case "ssh":
		add(sys.Getenv("SystemRoot"), "System32", "OpenSSH")
	case "winget":
		add(local, "Microsoft", "WindowsApps")
	}
	add(local, "Microsoft", "WinGet", "Links")
	add(sys.Getenv("ProgramFiles"), "WinGet", "Links")
	add(scoopRoot(sys), "shims")
	paths := make([]string, len(dirs))
	for i, dir := range dirs {
		paths[i] = winJoin(dir, name+".exe")
	}
	return paths
}

func scoopRoot(sys System) string {
	if dir := sys.Getenv("SCOOP"); dir != "" {
		return dir
	}
	if sys.Home == "" {
		return ""
	}
	return winJoin(sys.Home, "scoop")
}

// bundledAdb reports an adb that is the copy shipped inside scrcpy's Windows folder, which winget puts on PATH. Two
// adb versions on one machine keep restarting each other's server, so a platform-tools adb is preferred over it.
func bundledAdb(sys System, name, path string) bool {
	if sys.GOOS != "windows" || name != "adb" {
		return false
	}
	_, ok := sys.Stat(winJoin(winDir(path), "scrcpy.exe"))
	return ok
}

// windowsOrigin names the manager whose folder holds the binary, since Windows has no package database to ask. A
// winget link is followed to its package folder first. A binary in no known folder was unpacked by hand.
func windowsOrigin(sys System, path string) Origin {
	real, err := sys.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	local := sys.Getenv("LOCALAPPDATA")
	for _, root := range []string{
		winJoin(local, "Microsoft", "WinGet", "Packages"),
		winJoin(sys.Getenv("ProgramFiles"), "WinGet", "Packages"),
	} {
		if below, ok := winUnder(real, root); ok && len(below) > 1 {
			id, _, _ := strings.Cut(below[0], wingetSource)
			if id == below[0] {
				id, _, _ = strings.Cut(id, "_")
			}
			return Origin{Kind: OriginPackage, Package: id, Manager: ManagerWinget}
		}
	}
	gitRoots := []string{winJoin(local, "Programs", "Git")}
	for _, env := range []string{"ProgramFiles", "ProgramW6432"} {
		gitRoots = append(gitRoots, winJoin(sys.Getenv(env), "Git"))
	}
	for _, root := range gitRoots {
		if _, ok := winUnder(real, root); ok && len(winSegments(root)) > 1 {
			return Origin{Kind: OriginPackage, Package: "Git.Git", Manager: ManagerInstaller}
		}
	}
	if scoop := scoopRoot(sys); scoop != "" {
		if below, ok := winUnder(real, winJoin(scoop, "apps")); ok && len(below) > 1 {
			return Origin{Kind: OriginPackage, Package: below[0], Manager: ManagerScoop}
		}
		if below, ok := winUnder(real, winJoin(scoop, "shims")); ok && len(below) == 1 {
			return Origin{Kind: OriginPackage, Package: scoopShimApp(sys, real), Manager: ManagerScoop}
		}
	}
	if root := sys.Getenv("SystemRoot"); root != "" {
		if _, ok := winUnder(real, winJoin(root, "System32", "OpenSSH")); ok {
			return Origin{Kind: OriginPackage, Package: "OpenSSH.Client", Manager: ManagerWindows}
		}
	}
	return Origin{Kind: OriginManual}
}

// scoopShimApp reads which app a Scoop shim runs from the .shim file beside it, whose path line points into
// apps\<app>\. A shim it cannot read is named after its own file.
func scoopShimApp(sys System, shim string) string {
	name := strings.TrimSuffix(winBase(shim), ".exe")
	data, err := sys.ReadFile(winJoin(winDir(shim), name+".shim"))
	if err != nil {
		return name
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "path" {
			continue
		}
		target := strings.Trim(strings.TrimSpace(value), `"`)
		if below, ok := winUnder(target, winJoin(scoopRoot(sys), "apps")); ok && len(below) > 1 {
			return below[0]
		}
	}
	return name
}
