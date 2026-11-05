// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gitbash

import (
	"errors"
	"strings"
)

// ErrNotFound means no Git for Windows install was found.
var ErrNotFound = errors.New("gitbash: Git for Windows not found, install Git for Windows")

// find holds the lookup order, free of the OS so its tests run anywhere. CLAUDE_CODE_GIT_BASH_PATH comes first since
// it is how a user points Claude Code at a non-standard Git Bash, then git on PATH, then the two default installs.
// A candidate counts only if usr\bin\bash.exe exists under it, and nothing under the Windows directory, where WSL's
// bash.exe lives, is ever a root.
func find(getenv func(string) string, lookPath func(string) (string, error), isFile func(string) bool) (string, error) {
	var candidates []string
	if p := getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" {
		dir := winDir(p)
		candidates = append(candidates, winDir(dir))
		if strings.EqualFold(winBase(dir), "bin") && strings.EqualFold(winBase(winDir(dir)), "usr") {
			candidates = append(candidates, winDir(winDir(dir)))
		}
	}
	if git, err := lookPath("git.exe"); err == nil {
		dir := winDir(git)
		switch strings.ToLower(winBase(dir)) {
		case "cmd", "bin":
			candidates = append(candidates, winDir(dir))
		}
		if strings.EqualFold(winBase(dir), "bin") && strings.EqualFold(winBase(winDir(dir)), "mingw64") {
			candidates = append(candidates, winDir(winDir(dir)))
		}
	}
	if pf := getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, pf+`\Git`)
	}
	if local := getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates, local+`\Programs\Git`)
	}
	windows := strings.ToLower(getenv("SystemRoot"))
	for _, c := range candidates {
		if c == "" || (windows != "" && strings.HasPrefix(strings.ToLower(c), windows)) {
			continue
		}
		if isFile(c + `\usr\bin\bash.exe`) {
			return c, nil
		}
	}
	return "", ErrNotFound
}

// winDir and winBase split on either separator, since the inputs are Windows paths even when the tests run on Linux.
func winDir(p string) string {
	p = strings.TrimRight(p, `\/`)
	i := strings.LastIndexAny(p, `\/`)
	if i < 0 {
		return ""
	}
	return p[:i]
}

func winBase(p string) string {
	p = strings.TrimRight(p, `\/`)
	return p[strings.LastIndexAny(p, `\/`)+1:]
}
