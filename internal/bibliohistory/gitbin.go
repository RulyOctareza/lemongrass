// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package bibliohistory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// ErrNoGit means no usable git was found, so history is not recorded or read.
var ErrNoGit = errors.New("bibliohistory: git is not installed, so biblio history is off")

type gitLookup struct {
	goos     string
	lookPath func(string) (string, error)
	isFile   func(string) bool
	getenv   func(string) string
	// Whether the macOS Command Line Tools are installed.
	cltInstalled func() bool
}

const macGitStub = "/usr/bin/git"

// An app started from a desktop launcher, macOS Finder above all, gets a minimal PATH, so the usual install places
// are tried after it. On macOS /usr/bin/git is a stub that opens the Command Line Tools installer dialog when run,
// which a background daemon must never do, so it counts only once those tools are installed.
func findGit(l gitLookup) (string, error) {
	usable := func(p string) bool {
		return !(l.goos == "darwin" && p == macGitStub && !l.cltInstalled())
	}
	if p, err := l.lookPath("git"); err == nil && usable(p) {
		return p, nil
	}
	var candidates []string
	switch l.goos {
	case "darwin":
		candidates = []string{"/opt/homebrew/bin/git", "/usr/local/bin/git", macGitStub}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
			if dir := l.getenv(env); dir != "" {
				candidates = append(candidates, filepath.Join(dir, "Git", "cmd", "git.exe"))
			}
		}
		if dir := l.getenv("LOCALAPPDATA"); dir != "" {
			candidates = append(candidates, filepath.Join(dir, "Programs", "Git", "cmd", "git.exe"))
		}
	default:
		candidates = []string{"/usr/bin/git", "/usr/local/bin/git"}
	}
	for _, c := range candidates {
		if l.isFile(c) && usable(c) {
			return c, nil
		}
	}
	return "", ErrNoGit
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Replaced in tests to stand in for a machine without git.
var systemGit = func() gitLookup {
	return gitLookup{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
		isFile:   isFile,
		getenv:   os.Getenv,
		cltInstalled: func() bool {
			return exec.Command("xcode-select", "-p").Run() == nil
		},
	}
}

var (
	gitMu    sync.Mutex
	gitFound string
)

// GitPath finds git once and keeps it while it stays in place. A miss is not kept, so git installed while the app runs
// is picked up on the next call.
func GitPath() (string, error) {
	gitMu.Lock()
	defer gitMu.Unlock()
	if gitFound != "" && isFile(gitFound) {
		return gitFound, nil
	}
	p, err := findGit(systemGit())
	if err != nil {
		return "", err
	}
	gitFound = p
	return p, nil
}
