// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

func fakeHome(t *testing.T) (*keeper, string) {
	t.Helper()
	home := t.TempDir()
	lgrass := filepath.Join(home, ".lemongrass", "bin", "lgrassd")
	if err := os.MkdirAll(filepath.Dir(lgrass), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lgrass, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	k := newKeeper(home)
	k.candidates = []string{lgrass}
	return k, lgrass
}

func leaderSkillPath(k *keeper, v vendor) string {
	return filepath.Join(k.skillsRoot(v), "lgrass-howtobe-leader", "SKILL.md")
}

func embeddedLeaderSkill(t *testing.T, v vendor) string {
	t.Helper()
	data, err := skillsFS.ReadFile(v.embedRoot + "/lgrass-howtobe-leader/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInvalidSettingsAreNeverOverwritten(t *testing.T) {
	k, _ := fakeHome(t)
	if err := os.MkdirAll(filepath.Dir(k.claudeSettingsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `{"hooks": `
	if err := os.WriteFile(k.claudeSettingsPath(), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := k.reconcile(); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
	got, _ := os.ReadFile(k.claudeSettingsPath())
	if string(got) != broken {
		t.Errorf("file rewritten: %q", got)
	}
}

func TestWritePreservesFileMode(t *testing.T) {
	k, _ := fakeHome(t)
	if err := os.MkdirAll(filepath.Dir(k.claudeSettingsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k.claudeSettingsPath(), []byte(`{"editorMode":"normal"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(k.claudeSettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestNoLgrassBinaryRegistersNoHooks(t *testing.T) {
	k, lgrass := fakeHome(t)
	os.Remove(lgrass)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(k.claudeSettingsPath()); err == nil {
		t.Error("settings written although no lgrassd binary exists")
	}
	if _, err := os.Stat(leaderSkillPath(k, codex)); err != nil {
		t.Errorf("codex skill should still be installed: %v", err)
	}
	if _, err := os.Stat(k.codexHooksPath()); err == nil {
		t.Error("Codex hooks written although no lgrassd binary exists")
	}
}

func TestEachVendorGetsItsOwnSkillContent(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(leaderSkillPath(k, codex))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != embeddedLeaderSkill(t, codex) {
		t.Error("codex skill differs from its embedded copy")
	}
	if strings.Contains(embeddedLeaderSkill(t, codex), "allowed-tools") {
		t.Error("Codex skill carries a Claude-only header")
	}
}

func TestCodexInstallsEverySkillAndClaudeTheRetiredNone(t *testing.T) {
	k, _ := fakeHome(t)
	for _, name := range claude.retired {
		dir := filepath.Join(k.skillsRoot(claude), name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("old"), 0o644)
	}
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	if len(claude.retired) != 6 {
		t.Errorf("retired Claude skills = %v", claude.retired)
	}
	for _, name := range claude.retired {
		if _, err := os.Stat(filepath.Join(k.skillsRoot(claude), name)); err == nil {
			t.Errorf("retired Claude skill %s is still installed", name)
		}
	}
	for _, v := range []vendor{codex} {
		for _, name := range []string{"lgrass-howtobe-leader", "lgrass-howtobe-keeper", "lgrass-howtodo-taskclosing", "lgrass-howtodo-work", "lgrass-howtodo-newlaws", "lgrass-howtobe-thinker"} {
			if _, err := os.Stat(filepath.Join(k.skillsRoot(v), name, "SKILL.md")); err != nil {
				t.Errorf("%s skill %s missing: %v", v.configDir, name, err)
			}
		}
	}
}

func TestLegacySkillIsRemoved(t *testing.T) {
	k, _ := fakeHome(t)
	for _, v := range vendors {
		for _, name := range legacySkillNames {
			dir := k.legacySkillDir(v, name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	extra := filepath.Join(k.legacySkillDir(codex, "lemongrass"), "notes.txt")
	if err := os.WriteFile(extra, []byte("user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, v := range vendors {
		for _, name := range legacySkillNames {
			if _, err := os.Stat(filepath.Join(k.legacySkillDir(v, name), "SKILL.md")); err == nil {
				t.Errorf("%s legacy %s SKILL.md still present", v.configDir, name)
			}
		}
	}
	if _, err := os.Stat(leaderSkillPath(k, codex)); err != nil {
		t.Errorf("codex leader skill missing: %v", err)
	}
	for _, name := range legacySkillNames {
		if _, err := os.Stat(k.legacySkillDir(claude, name)); err == nil {
			t.Errorf("empty legacy Claude directory %s still present", name)
		}
	}
	if _, err := os.Stat(extra); err != nil {
		t.Errorf("unrelated file in the legacy directory was removed: %v", err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

// watchedKeeper runs a keeper's watcher for one test and counts its started and finished passes.
type watchedKeeper struct {
	started, finished atomic.Int64
}

// startWatcher runs k's watcher until the test ends.
func startWatcher(t *testing.T, k *keeper) *watchedKeeper {
	t.Helper()
	w := &watchedKeeper{}
	k.onPass = func(finished bool) {
		if finished {
			w.finished.Add(1)
		} else {
			w.started.Add(1)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return w
}

// waitFor waits until cond holds with at least one pass done and none in flight. A pass writes files before it arms its watches, so a test that changed a file as soon as the file looked right could change it before the watch on it was armed, which kqueue does slowly, and the change would go unseen until the safety tick.
func (w *watchedKeeper) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	waitFor(t, what, func() bool {
		finished := w.finished.Load()
		return finished > 0 && cond() && w.started.Load() == finished
	})
}

func TestWatcherHealsDriftWithoutAnyOtherProcess(t *testing.T) {
	k, lgrass := fakeHome(t)
	w := startWatcher(t, k)

	healthy := func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), lgrass+" hook PreToolUse") && !strings.Contains(string(data), "Write|Edit")
	}
	w.waitFor(t, "initial registration", healthy)

	drifted := `{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"` + lgrass + ` hook PreToolUse"}]}]}}`
	if err := agentconf.WriteAtomic(k.claudeSettingsPath(), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "matcher healed after drift", healthy)

	if err := os.RemoveAll(filepath.Dir(leaderSkillPath(k, codex))); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "skill restored after deletion", func() bool {
		data, err := os.ReadFile(leaderSkillPath(k, codex))
		return err == nil && string(data) == embeddedLeaderSkill(t, codex)
	})
	w.waitFor(t, "Codex hooks registered", func() bool {
		data, err := os.ReadFile(k.codexHooksPath())
		return err == nil && strings.Contains(string(data), "LGRASS_HOOK_VENDOR=codex "+lgrass+" hook PreToolUse")
	})
}

func TestWatcherRegistersHooksWhenLgrassAppears(t *testing.T) {
	k, lgrass := fakeHome(t)
	os.Remove(lgrass)
	w := startWatcher(t, k)

	w.waitFor(t, "skill installed", func() bool { _, err := os.Stat(leaderSkillPath(k, codex)); return err == nil })
	if _, err := os.Stat(k.claudeSettingsPath()); err == nil {
		t.Fatal("settings written before the binary existed")
	}
	if err := os.WriteFile(lgrass, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "hooks registered after binary appeared", func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), lgrass+" hook SessionStart")
	})
	w.waitFor(t, "Codex hooks registered after binary appeared", func() bool {
		data, err := os.ReadFile(k.codexHooksPath())
		return err == nil && strings.Contains(string(data), "LGRASS_HOOK_VENDOR=codex "+lgrass+" hook SessionStart")
	})
}

func TestStatusLineChainedForPrimaryAndAccounts(t *testing.T) {
	k, lgrass := fakeHome(t)
	os.MkdirAll(filepath.Dir(k.claudeSettingsPath()), 0o755)
	os.WriteFile(k.claudeSettingsPath(), []byte(`{"statusLine":{"type":"command","command":"line.sh"}}`), 0o644)
	accounts := k.accounts()
	os.WriteFile(accounts.FilePath(), []byte(`{"accounts":["work"]}`), 0o600)
	store := accounts.StatusLines()
	store.Prune(nil)
	os.WriteFile(store.Path, []byte(`{"commands":{"gone":"x.sh"}}`), 0o600)

	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ path, account string }{
		{k.claudeSettingsPath(), "primary"},
		{accounts.SettingsPath("work"), "work"},
	} {
		data, _ := os.ReadFile(c.path)
		if !strings.Contains(string(data), agentconf.StatusLineCommand(lgrass, c.account)) {
			t.Errorf("%s: statusline not chained: %s", c.account, data)
		}
	}
	if got, _ := store.Command("primary"); got != "line.sh" {
		t.Errorf("primary command stored as %q", got)
	}
	if got, _ := store.Command("gone"); got != "" {
		t.Error("entry of a removed account kept")
	}
}

func TestStatusLineOffSwitchRestoresAndOnRetakes(t *testing.T) {
	k, lgrass := fakeHome(t)
	os.MkdirAll(filepath.Dir(k.claudeSettingsPath()), 0o755)
	os.WriteFile(k.claudeSettingsPath(), []byte(`{"statusLine":{"type":"command","command":"line.sh"}}`), 0o644)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	ours := agentconf.StatusLineCommand(lgrass, "primary")

	off := agentconf.StatusLineOffPath(k.home)
	os.WriteFile(off, nil, 0o600)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(k.claudeSettingsPath())
	if strings.Contains(string(data), ours) || !strings.Contains(string(data), "line.sh") {
		t.Fatalf("off: %s", data)
	}
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(k.claudeSettingsPath())
	if strings.Contains(string(data), ours) {
		t.Fatal("off: statusline retaken on a later pass")
	}

	os.Remove(off)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(k.claudeSettingsPath())
	if !strings.Contains(string(data), ours) {
		t.Errorf("on: statusline not retaken: %s", data)
	}
	if got, _ := k.accounts().StatusLines().Command("primary"); got != "line.sh" {
		t.Errorf("on: human command stored as %q", got)
	}
}
