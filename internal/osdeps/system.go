// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// FileStat is what the check needs to know about a file.
type FileStat struct {
	Size    int64
	ModTime int64
	Dir     bool
	Exec    bool
}

// System holds every effect the check has on the outside world, so tests replace it whole with fakes.
type System struct {
	GOOS string
	Home string
	// Run returns the command's stdout and stderr together, since ssh prints its version on stderr.
	Run          func(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath     func(name string) (string, error)
	Stat         func(path string) (FileStat, bool)
	EvalSymlinks func(path string) (string, error)
	ReadFile     func(path string) ([]byte, error)
	Getenv       func(key string) string
}

const commandTimeout = 5 * time.Second

// Real is the System of the running machine.
func Real() System {
	home, _ := os.UserHomeDir()
	return System{
		GOOS:         runtime.GOOS,
		Home:         home,
		Run:          runCommand,
		LookPath:     lookPath,
		Stat:         statFile,
		EvalSymlinks: filepath.EvalSymlinks,
		ReadFile:     os.ReadFile,
		Getenv:       os.Getenv,
	}
}

// The C locale keeps rpm's "is not owned" and every tool's version line in English, which the parsers read.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	return cmd.CombinedOutput()
}

func statFile(path string) (FileStat, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return FileStat{}, false
	}
	return FileStat{
		Size:    info.Size(),
		ModTime: info.ModTime().UnixNano(),
		Dir:     info.IsDir(),
		Exec:    runtime.GOOS == "windows" || info.Mode()&0o111 != 0,
	}, true
}
