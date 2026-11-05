// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"regexp"
	"strings"
)

// The functions in this file turn the Windows spellings of a path or a command name into the one form the rules compare. They are plain string code with no path/filepath and no syscalls, so their tests run on every OS; normalize_windows.go wires them in front of the rules and normalize_other.go keeps every other OS on identity.

type spellingKind int

const (
	spellingOK spellingKind = iota
	// spellingVariable is a path holding an environment variable the gate does not resolve, judged like a path built from $VAR.
	spellingVariable
	// spellingUnmappable is a form the gate cannot reduce to a drive path, such as a volume GUID; it is denied.
	spellingUnmappable
)

// winEnv is what a Windows spelling is resolved against. Home is not in it: the home spellings become ~, which ResolvePath expands against the home the caller passes.
type winEnv struct {
	AppData      string
	LocalAppData string
	// CwdDrive is the drive of the shell's directory, such as C:, for a rooted \x and a drive-relative c:x.
	CwdDrive string
	// GitRoot is the Git for Windows install, where Git Bash mounts /, /etc and /usr. Empty when unknown.
	GitRoot string
	TempDir string
}

var (
	homeSpellings    = regexp.MustCompile(`(?i)^(%USERPROFILE%|%HOMEDRIVE%%HOMEPATH%|\$env:USERPROFILE|\$\{env:USERPROFILE\}|\$USERPROFILE|\$HOMEDRIVE\$HOMEPATH)(/|$)`)
	appDataSpellings = regexp.MustCompile(`(?i)^(%APPDATA%|\$env:APPDATA|\$\{env:APPDATA\}|\$APPDATA)(/|$)`)
	localSpellings   = regexp.MustCompile(`(?i)^(%LOCALAPPDATA%|\$env:LOCALAPPDATA|\$\{env:LOCALAPPDATA\}|\$LOCALAPPDATA)(/|$)`)
	leftoverVariable = regexp.MustCompile(`(?i)%[a-z_][a-z0-9_()]*%|\$\{?env:`)
	// A device namespace path names a disk or volume directly; these are kept as they are, so the raw-device rules can see them.
	rawDevicePath = regexp.MustCompile(`(?i)^//\./(physicaldrive[0-9]+|harddisk[0-9]+|harddiskvolume[0-9]+|cdrom[0-9]+|tape[0-9]+|[a-z]:|nul|con|prn|aux)(/|$)`)
	// An 8.3 short name is up to six characters, a tilde and a number, with an optional extension of up to three.
	shortNameSegment = regexp.MustCompile(`(?i)(^|/)[^/.~]{1,6}~[0-9]+(\.[^/.]{0,3})?(/|$)`)
	driveLetterPath  = regexp.MustCompile(`^[A-Za-z]:`)
)

// windowsSpelling rewrites a path as Windows spells it into forward slashes with a drive letter, or into a relative path or a ~ path that ResolvePath anchors. It does not change case; folding is done after resolution.
func windowsSpelling(p string, env winEnv) (string, spellingKind) {
	p = strings.TrimSpace(p)
	if p == "" {
		return p, spellingOK
	}
	// A rooted \x is on the current drive. Checked before the slashes are turned, since Git Bash maps a rooted /x under its own install instead.
	if strings.HasPrefix(p, `\??\`) {
		return strings.ReplaceAll(p, `\`, "/"), spellingUnmappable
	}
	if strings.HasPrefix(p, `\`) && !strings.HasPrefix(p, `\\`) && env.CwdDrive != "" {
		p = env.CwdDrive + p
	}
	p = strings.ReplaceAll(p, `\`, "/")
	p = expandWindowsVariables(p, env)
	if leftoverVariable.MatchString(p) {
		return p, spellingVariable
	}
	p, kind, done := windowsPrefix(p, env)
	if done || kind != spellingOK {
		return p, kind
	}
	return trimSegments(p), spellingOK
}

func expandWindowsVariables(p string, env winEnv) string {
	if m := homeSpellings.FindStringSubmatchIndex(p); m != nil {
		return "~" + p[m[3]:]
	}
	if env.AppData != "" {
		if m := appDataSpellings.FindStringSubmatchIndex(p); m != nil {
			return strings.ReplaceAll(env.AppData, `\`, "/") + p[m[3]:]
		}
	}
	if env.LocalAppData != "" {
		if m := localSpellings.FindStringSubmatchIndex(p); m != nil {
			return strings.ReplaceAll(env.LocalAppData, `\`, "/") + p[m[3]:]
		}
	}
	return p
}

// windowsPrefix maps the leading part of a slash path to a drive path. done means the result is final, such as a device path, which takes no segment trimming.
func windowsPrefix(p string, env winEnv) (string, spellingKind, bool) {
	lower := strings.ToLower(p)
	switch {
	case strings.HasPrefix(lower, "//?/unc/"):
		return "//" + p[len("//?/unc/"):], spellingOK, false
	case strings.HasPrefix(lower, "//?/") || strings.HasPrefix(lower, "//./"):
		rest := p[4:]
		if driveLetterPath.MatchString(rest) && (len(rest) == 2 || rest[2] == '/') && strings.HasPrefix(lower, "//?/") {
			return rest, spellingOK, false
		}
		if rawDevicePath.MatchString(p) {
			return strings.ToLower(p), spellingOK, true
		}
		// A volume GUID, GLOBALROOT or any other object path cannot be tied to a drive path.
		if rest != "" && rest[0] != '/' {
			return p, spellingUnmappable, true
		}
	case strings.HasPrefix(lower, "/??/"):
		return p, spellingUnmappable, true
	case strings.HasPrefix(p, "//"):
		return p, spellingOK, false
	case strings.HasPrefix(p, "/"):
		return gitBashPath(p, env), spellingOK, false
	case driveLetterPath.MatchString(p) && (len(p) == 2 || p[2] != '/'):
		// A drive-relative c:x runs from that drive's current directory, known only for the shell's own drive; on another drive it is placed at the root, which can only add verdicts.
		if strings.EqualFold(p[:2], env.CwdDrive) {
			rest := p[2:]
			if rest == "" {
				rest = "."
			}
			return rest, spellingOK, false
		}
		return p[:2] + "/" + p[2:], spellingOK, false
	}
	return p, spellingOK, false
}

// gitBashPath maps a Git Bash absolute path: /c/x and /cygdrive/c/x are drive C, /dev stays a device, /tmp is the user's temp dir, and anything else lives under the Git for Windows install.
func gitBashPath(p string, env winEnv) string {
	lower := strings.ToLower(p)
	for _, prefix := range []string{"/cygdrive/", "/"} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if len(rest) >= 1 && isLetter(rest[0]) && (len(rest) == 1 || rest[1] == '/') {
			return strings.ToUpper(rest[:1]) + ":/" + strings.TrimPrefix(rest[1:], "/")
		}
	}
	switch {
	case lower == "/dev" || strings.HasPrefix(lower, "/dev/"):
		return p
	case (lower == "/tmp" || strings.HasPrefix(lower, "/tmp/")) && env.TempDir != "":
		return strings.TrimRight(strings.ReplaceAll(env.TempDir, `\`, "/"), "/") + p[len("/tmp"):]
	case env.GitRoot != "":
		return strings.TrimRight(strings.ReplaceAll(env.GitRoot, `\`, "/"), "/") + p
	case env.CwdDrive != "":
		return env.CwdDrive + p
	}
	return p
}

// trimSegments drops what Windows itself ignores in a name: an alternate data stream after a colon, and trailing dots and spaces, so .ssh::$DATA and ".ssh." both reach .ssh.
func trimSegments(p string) string {
	prefix := ""
	switch {
	case driveLetterPath.MatchString(p):
		prefix, p = p[:2], p[2:]
	case strings.HasPrefix(p, "//"):
		// The server and share of a UNC path are names, not files, and keep their colons.
		parts := strings.SplitN(p[2:], "/", 3)
		if len(parts) < 3 {
			return p
		}
		prefix, p = "//"+parts[0]+"/"+parts[1], "/"+parts[2]
	}
	segments := strings.Split(p, "/")
	for i, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || (i == 0 && strings.HasPrefix(segment, "~")) {
			continue
		}
		if colon := strings.IndexByte(segment, ':'); colon > 0 {
			segment = segment[:colon]
		}
		if trimmed := strings.TrimRight(segment, ". "); trimmed != "" {
			segment = trimmed
		}
		segments[i] = segment
	}
	return prefix + strings.Join(segments, "/")
}

// windowsCommandName is the name a rule compares for a command written as a Windows path: the last component of either slash, lowercased, without an executable extension, so RM.EXE and C:\Git\usr\bin\rm.exe are rm.
func windowsCommandName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 && i < len(name)-1 {
		name = name[i+1:]
	}
	name = strings.TrimRight(strings.ToLower(name), ". ")
	for _, ext := range []string{".exe", ".com", ".cmd", ".bat"} {
		if strings.HasSuffix(name, ext) && len(name) > len(ext) {
			return strings.TrimSuffix(name, ext)
		}
	}
	return name
}

// hasShortNameSegment reports a component that looks like an 8.3 short name, such as PROGRA~1, which names a long-named file only the filesystem can tell.
func hasShortNameSegment(p string) bool {
	return shortNameSegment.MatchString(strings.ReplaceAll(p, `\`, "/"))
}

// unescapeDoubleQuotedBash removes the backslashes bash removes inside double quotes: only before $, a backtick, ", \ or a newline. Every other backslash stays, so "C:\Users\x" keeps its separators.
func unescapeDoubleQuotedBash(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			switch value[i+1] {
			case '\n':
				i++
				continue
			case '$', '`', '"', '\\':
				i++
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
