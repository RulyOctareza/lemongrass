// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package guard

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/faizalv/lemongrass/internal/gitbash"
)

// Windows spells one path many ways (drive letters, Git Bash /c/, %USERPROFILE%, \\?\, short names, any case) and its filesystem ignores case, so every path and command name is reduced here to one lowercase form before the rules see it. The rules themselves never branch on the OS; normalize_other.go is identity.

// spellPath rewrites a written path through windowsSpelling against this machine.
func spellPath(p, cwd string) (string, spellingKind) {
	return windowsSpelling(p, currentWinEnv(cwd))
}

// foldPath is the last step of every resolved path: a short-name component is expanded when the file exists, then the path is lowercased, since NTFS compares names without case.
func foldPath(p string) string {
	if hasShortNameSegment(p) {
		if long, ok := longPathName(p); ok {
			p = long
		}
	}
	return strings.ToLower(p)
}

func foldText(s string) string { return strings.ToLower(s) }

func commandBase(name string) string { return windowsCommandName(name) }

func namesPath(name string) bool { return strings.ContainsAny(name, `/\`) }

func unescapeDoubleQuoted(value string) string { return unescapeDoubleQuotedBash(value) }

// pathLiteral reports whether a written path names one place the gate can know before the command runs. A variable it does not resolve, an unmappable form or a short name that names no existing file is judged like a path built from $VAR.
func pathLiteral(p string) bool {
	spelled, kind := spellPath(p, "")
	if kind != spellingOK {
		return false
	}
	if !hasShortNameSegment(spelled) {
		return true
	}
	abs, err := filepath.Abs(filepath.FromSlash(spelled))
	if err != nil {
		return false
	}
	_, ok := longPathName(abs)
	return ok
}

// globPath expands a pattern without case, as the shell does on a case-insensitive filesystem; filepath.Glob would compare the folded pattern with case.
func globPath(pattern string) ([]string, error) {
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(pattern)
	rest := pattern[len(volume):]
	base := volume
	if strings.HasPrefix(rest, `\`) {
		base += `\`
	}
	if base == "" {
		base = "."
	}
	paths := []string{base}
	for _, segment := range strings.Split(rest, `\`) {
		if segment == "" {
			continue
		}
		var next []string
		for _, dir := range paths {
			if !strings.ContainsAny(segment, "*?[") {
				next = append(next, filepath.Join(dir, segment))
				continue
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if ok, _ := filepath.Match(strings.ToLower(segment), strings.ToLower(entry.Name())); ok {
					next = append(next, filepath.Join(dir, entry.Name()))
				}
			}
		}
		paths = next
	}
	var matches []string
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

// platformSystemDirs are the Windows directories whose removal is catastrophic, folded like any resolved path.
func platformSystemDirs() []string {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	dirs := []string{drive + `\`, drive + `\Users`}
	for _, name := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramData", "ProgramW6432"} {
		if dir := os.Getenv(name); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	for i, dir := range dirs {
		dirs[i] = strings.ToLower(filepath.Clean(dir))
	}
	return dirs
}

func currentWinEnv(cwd string) winEnv {
	drive := filepath.VolumeName(cwd)
	if drive == "" {
		drive = os.Getenv("SystemDrive")
	}
	return winEnv{
		AppData:      os.Getenv("APPDATA"),
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		CwdDrive:     drive,
		GitRoot:      gitRoot(),
		TempDir:      os.TempDir(),
	}
}

// gitRoot is the Git for Windows install Git Bash mounts as /, or "" when none is found.
func gitRoot() string {
	root, err := gitbash.Root()
	if err != nil {
		return ""
	}
	return root
}

// longPathName expands short names in an existing path; it fails for a path that does not exist.
func longPathName(p string) (string, bool) {
	from, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, syscall.MAX_PATH)
	for {
		n, err := syscall.GetLongPathName(from, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			return "", false
		}
		if int(n) < len(buf) {
			return syscall.UTF16ToString(buf[:n]), true
		}
		buf = make([]uint16, n)
	}
}
