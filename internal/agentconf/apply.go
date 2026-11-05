// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"bytes"
	"errors"
	"os"
	"time"

	"github.com/faizalv/lemongrass/internal/atomicfile"
)

func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	return atomicfile.Write(path, data, mode)
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

// RemoveHookConfig takes the Lemongrass hook groups out of one settings file and reports whether the file changed.
// With deleteIfEmpty, a file left with nothing in it is deleted instead of written back as an empty object.
func RemoveHookConfig(path string, events []string, deleteIfEmpty bool) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	out, changed, empty, err := RemoveHookGroups(data, events)
	if err != nil {
		return false, err
	}
	if deleteIfEmpty && empty {
		if err := os.Remove(path); err != nil {
			return false, err
		}
		return true, nil
	}
	if !changed {
		return false, nil
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return true, WriteAtomic(path, out, mode)
}
