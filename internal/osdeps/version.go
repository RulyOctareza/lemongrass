// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"regexp"
	"strconv"
	"strings"
)

// Each tool prints its version its own way; the first pattern that matches wins. Suffixes such as Fedora's "p1",
// Apple's "(Apple Git-154)" or ".windows.1" stay out of the captured number.
var versionPatterns = map[string][]*regexp.Regexp{
	"git": {regexp.MustCompile(`git version (\d+\.\d+(?:\.\d+)?)`)},
	// The "Version" line is the platform-tools release; the first line's 1.0.41 is adb's protocol version.
	"adb": {
		regexp.MustCompile(`(?m)^Version (\d+\.\d+\.\d+)`),
		regexp.MustCompile(`Android Debug Bridge version (\d+\.\d+\.\d+)`),
	},
	"scrcpy": {regexp.MustCompile(`(?m)^scrcpy v?(\d+\.\d+(?:\.\d+)?)`)},
	// Release builds print "7.1.5" or "n7.1"; a nightly prints "N-12345-g<hash>" and has no version.
	"ffmpeg": {regexp.MustCompile(`ffmpeg version n?(\d+\.\d+(?:\.\d+)?)`)},
	// Windows' own client prints "OpenSSH_for_Windows_9.5p1".
	"ssh": {regexp.MustCompile(`OpenSSH_(?:for_Windows_)?(\d+\.\d+(?:p\d+)?)`)},
}

// ParseVersion reads a tool's version from its output, or returns "" when it cannot.
func ParseVersion(name, out string) string {
	for _, re := range versionPatterns[name] {
		if m := re.FindStringSubmatch(out); m != nil {
			return m[1]
		}
	}
	return ""
}

// CompareVersions compares dotted versions part by part as numbers, reading only each part's leading digits, so
// "10.0p2" compares as 10.0. A missing part counts as 0.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		na, nb := leadingNumber(pa, i), leadingNumber(pb, i)
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func leadingNumber(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	end := 0
	for end < len(parts[i]) && parts[i][end] >= '0' && parts[i][end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(parts[i][:end])
	return n
}
