// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package flutter runs a Flutter project's app and tests for lgrass flutter.
package flutter

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindProject walks up from dir to the directory holding pubspec.yaml.
func FindProject(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "pubspec.yaml")); err == nil {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("flutter: no pubspec.yaml in %s or above it, run this inside a Flutter project", abs)
		}
	}
}

// SDK returns the flutter binary for a project. A project pinned with FVM gets its own SDK,
// so tests and runs use the version the project builds with.
func SDK(project string) (string, error) {
	pinned := filepath.Join(project, ".fvm", "flutter_sdk", "bin", "flutter")
	if info, err := os.Stat(pinned); err == nil && !info.IsDir() {
		return pinned, nil
	}
	if path, err := exec.LookPath("flutter"); err == nil {
		return path, nil
	}
	return "", errors.New("flutter: the flutter command was not found on PATH and the project has no .fvm/flutter_sdk")
}

// StateDir is where a project's run and test artifacts live. It sits outside the project so
// nothing lgrass writes shows up in the project's git status.
func StateDir(project string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("flutter: %w", err)
	}
	sum := sha1.Sum([]byte(project))
	dir := filepath.Join(base, "lemongrass", "flutter", filepath.Base(project)+"-"+hex.EncodeToString(sum[:])[:10])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("flutter: %w", err)
	}
	return dir, nil
}
