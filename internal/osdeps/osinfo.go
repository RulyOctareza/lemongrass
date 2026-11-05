// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// OS families. A family decides how origin is read and which route an offer takes.
const (
	FamilyFedora  = "fedora"
	FamilyDebian  = "debian"
	FamilyMacOS   = "macos"
	FamilyWindows = "windows"
	FamilyUnknown = "unknown"
)

// OSInfo is the OS the check ran on.
type OSInfo struct {
	Family  string `json:"family"`
	ID      string `json:"id"`
	Version string `json:"version"`
	// An image-based Fedora such as Silverblue, where dnf cannot install and tools come with the image or by
	// rpm-ostree layering.
	Atomic bool `json:"-"`
	// Windows only: the build number, and whether winget is there to install with.
	Build  int  `json:"-"`
	Winget bool `json:"-"`
}

// DetectOS reads the OS and its release.
func DetectOS(ctx context.Context, sys System) OSInfo {
	switch sys.GOOS {
	case "darwin":
		out, _ := sys.Run(ctx, "sw_vers", "-productVersion")
		return OSInfo{Family: FamilyMacOS, ID: "macos", Version: strings.TrimSpace(string(out))}
	case "windows":
		out, _ := sys.Run(ctx, "cmd", "/c", "ver")
		info := ParseWindowsVer(string(out))
		info.Winget = Resolve(ctx, sys, "winget") != ""
		return info
	case "linux":
		data, err := sys.ReadFile("/etc/os-release")
		if err != nil {
			data, err = sys.ReadFile("/usr/lib/os-release")
		}
		if err != nil {
			return OSInfo{Family: FamilyUnknown, ID: "linux"}
		}
		info := ParseOSRelease(string(data))
		_, info.Atomic = sys.Stat("/run/ostree-booted")
		return info
	}
	return OSInfo{Family: FamilyUnknown, ID: sys.GOOS}
}

// ParseOSRelease reads the family, id and release from the text of /etc/os-release.
func ParseOSRelease(data string) OSInfo {
	fields := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		fields[key] = strings.Trim(value, `"'`)
	}
	info := OSInfo{Family: FamilyUnknown, ID: fields["ID"], Version: fields["VERSION_ID"]}
	// ID first, so Mint (ID_LIKE "ubuntu debian") and Rocky (ID_LIKE "rhel centos fedora") land the same way their
	// parent does.
	for _, id := range append([]string{fields["ID"]}, strings.Fields(fields["ID_LIKE"])...) {
		switch id {
		case "fedora", "rhel", "centos":
			info.Family = FamilyFedora
			return info
		case "debian", "ubuntu":
			info.Family = FamilyDebian
			return info
		}
	}
	return info
}

// The bracketed version is the one part of cmd's ver line that is not translated.
var windowsVer = regexp.MustCompile(`\[[^\]]*?(\d+)\.(\d+)\.(\d+)`)

// windowsBuild11 is the first build of Windows 11, which still reports itself as version 10.0.
const windowsBuild11 = 22000

// ParseWindowsVer reads the release and build from the output of cmd /c ver, such as
// "Microsoft Windows [Version 10.0.22631.4460]".
func ParseWindowsVer(out string) OSInfo {
	info := OSInfo{Family: FamilyWindows, ID: "windows"}
	m := windowsVer.FindStringSubmatch(out)
	if m == nil {
		return info
	}
	info.Build, _ = strconv.Atoi(m[3])
	release := m[1]
	if release == "10" && info.Build >= windowsBuild11 {
		release = "11"
	}
	info.Version = release + " (build " + m[3] + ")"
	return info
}
