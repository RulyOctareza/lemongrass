// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package gitbash

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Root is the Git for Windows install root, the directory holding usr, etc and bin. Cached for the process.
func Root() (string, error) {
	return root()
}

// Bash is the full path of Git Bash's bash.exe. bin\bash.exe comes first: it is the launcher that puts usr\bin and
// mingw64\bin on PATH, which a bare usr\bin\bash.exe -c run from a Windows process does not have.
func Bash() (string, error) {
	r, err := root()
	if err != nil {
		return "", err
	}
	if launcher := filepath.Join(r, "bin", "bash.exe"); isFile(launcher) {
		return launcher, nil
	}
	return filepath.Join(r, "usr", "bin", "bash.exe"), nil
}

var root = sync.OnceValues(func() (string, error) {
	return find(os.Getenv, exec.LookPath, isFile)
})

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
