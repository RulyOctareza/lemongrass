// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package agentconf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".lgrassconf-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func statKey(path string) (time.Time, int64) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, -1
	}
	return info.ModTime(), info.Size()
}

type HookReconciler func([]byte, string) ([]byte, bool, error)

func SyncHookConfig(path, lgrassdPath string, reconcile HookReconciler) error {
	for attempt := 0; attempt < 3; attempt++ {
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
		beforeTime, beforeSize := statKey(path)

		out, changed, err := reconcile(data, lgrassdPath)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		afterTime, afterSize := statKey(path)
		if !afterTime.Equal(beforeTime) || afterSize != beforeSize {
			continue
		}
		return WriteAtomic(path, out, mode)
	}
	return errors.New("settings changed on every attempt, giving up until the next event")
}

func SyncFile(path string, want []byte) error {
	have, err := os.ReadFile(path)
	if err == nil && bytes.Equal(have, want) {
		return nil
	}
	return WriteAtomic(path, want, 0o644)
}
