// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

// installedBinaries are the names the app copies into a bin folder on PATH; lgrassd lives in the keeper's candidates.
var installedBinaries = []string{"lgrass", "lgrassconf"}

// binDirs are where the app may have copied lgrass and lgrassconf: ~/.local/bin, or /usr/local/bin when that folder
// was missing (resolveInstallDir in ui/src/main/lgrassInstall.ts).
func binDirs(home string) []string {
	return []string{filepath.Join(home, ".local", "bin"), "/usr/local/bin"}
}

// uninstall reverses what install, the keeper and the app put outside the lemongrass home. The service goes first,
// because a running keeper puts the config back within moments of any change. Every step reports what it removed,
// and a failing step is reported while the rest still run. The lemongrass home, the Claude account folders and the
// project trust entries stay.
func (k *keeper) uninstall(out io.Writer, removeService func(home string) error, dirs []string) error {
	var errs []error
	fail := func(what string, err error) {
		fmt.Fprintf(out, "failed: %s: %v\n", what, err)
		errs = append(errs, fmt.Errorf("lgrassconf: %s: %w", what, err))
	}

	if err := removeService(k.home); err != nil {
		fail("removing the keeper service", err)
	} else {
		fmt.Fprintln(out, "removed the keeper service")
	}

	accounts := k.accounts()
	store := accounts.StatusLines()
	settings := []struct{ path, account string }{{k.claudeSettingsPath(), "primary"}}
	names, err := accounts.Names()
	if err != nil {
		fail("reading the Claude accounts", err)
	}
	for _, name := range names {
		settings = append(settings, struct{ path, account string }{accounts.SettingsPath(name), name})
	}
	for _, s := range settings {
		before, _ := os.ReadFile(s.path)
		if _, err := agentconf.RemoveHookConfig(s.path, agentconf.ClaudeHookEvents, false); err != nil {
			fail("removing the hooks from "+s.path, err)
		}
		if err := agentconf.SyncStatusLine(s.path, "", s.account, store); err != nil {
			fail("restoring the status line in "+s.path, err)
		}
		if after, _ := os.ReadFile(s.path); !bytes.Equal(before, after) {
			fmt.Fprintf(out, "cleaned %s\n", s.path)
		}
	}

	if removed, err := agentconf.RemoveHookConfig(k.codexHooksPath(), codexHookEvents, true); err != nil {
		fail("removing the hooks from "+k.codexHooksPath(), err)
	} else if removed {
		fmt.Fprintf(out, "cleaned %s\n", k.codexHooksPath())
	}

	var skillDirs []string
	for _, f := range k.skills {
		skillDirs = append(skillDirs, filepath.Dir(f.path))
	}
	for _, v := range vendors {
		for _, name := range v.removedSkills() {
			skillDirs = append(skillDirs, k.legacySkillDir(v, name))
		}
	}
	for _, dir := range skillDirs {
		if !exists(filepath.Join(dir, "SKILL.md")) {
			continue
		}
		if err := removeLegacySkill(dir); err != nil {
			fail("removing the skill "+dir, err)
		} else if exists(dir) {
			fmt.Fprintf(out, "removed %s, other files in its folder stay\n", filepath.Join(dir, "SKILL.md"))
		} else {
			fmt.Fprintf(out, "removed %s\n", dir)
		}
	}

	binaries := append([]string{}, k.candidates...)
	for _, dir := range dirs {
		for _, name := range installedBinaries {
			binaries = append(binaries, filepath.Join(dir, name))
		}
	}
	for _, path := range binaries {
		if !exists(path) {
			continue
		}
		if err := os.Remove(path); err != nil {
			fail("removing "+path, err)
		} else {
			fmt.Fprintf(out, "removed %s\n", path)
		}
	}
	return errors.Join(errs...)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}
