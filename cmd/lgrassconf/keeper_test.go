// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	for _, v := range vendors {
		if _, err := os.Stat(leaderSkillPath(k, v)); err != nil {
			t.Errorf("%s skill should still be installed: %v", v.configDir, err)
		}
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
	for _, v := range vendors {
		got, err := os.ReadFile(leaderSkillPath(k, v))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != embeddedLeaderSkill(t, v) {
			t.Errorf("%s skill differs from its embedded copy", v.configDir)
		}
	}
	if !strings.Contains(embeddedLeaderSkill(t, claude), "allowed-tools: Bash(lgrass *)") {
		t.Error("Claude skill lost its allowed-tools header")
	}
	if strings.Contains(embeddedLeaderSkill(t, codex), "allowed-tools") {
		t.Error("Codex skill carries a Claude-only header")
	}
}

func TestEveryVendorInstallsEverySkill(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, v := range vendors {
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
		if _, err := os.Stat(leaderSkillPath(k, v)); err != nil {
			t.Errorf("%s leader skill missing: %v", v.configDir, err)
		}
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

func TestWatcherHealsDriftWithoutAnyOtherProcess(t *testing.T) {
	k, lgrass := fakeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	healthy := func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), lgrass+" hook PreToolUse") && !strings.Contains(string(data), "Write|Edit")
	}
	waitFor(t, "initial registration", healthy)

	drifted := `{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"` + lgrass + ` hook PreToolUse"}]}]}}`
	if err := agentconf.WriteAtomic(k.claudeSettingsPath(), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "matcher healed after drift", healthy)

	if err := os.RemoveAll(filepath.Dir(leaderSkillPath(k, claude))); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "skill restored after deletion", func() bool {
		data, err := os.ReadFile(leaderSkillPath(k, claude))
		return err == nil && string(data) == embeddedLeaderSkill(t, claude)
	})
	waitFor(t, "Codex hooks registered", func() bool {
		data, err := os.ReadFile(k.codexHooksPath())
		return err == nil && strings.Contains(string(data), "LGRASS_HOOK_VENDOR=codex "+lgrass+" hook PreToolUse")
	})
}

func TestWatcherRegistersHooksWhenLgrassAppears(t *testing.T) {
	k, lgrass := fakeHome(t)
	os.Remove(lgrass)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitFor(t, "skill installed", func() bool { _, err := os.Stat(leaderSkillPath(k, claude)); return err == nil })
	if _, err := os.Stat(k.claudeSettingsPath()); err == nil {
		t.Fatal("settings written before the binary existed")
	}
	if err := os.WriteFile(lgrass, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "hooks registered after binary appeared", func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), lgrass+" hook SessionStart")
	})
	waitFor(t, "Codex hooks registered after binary appeared", func() bool {
		data, err := os.ReadFile(k.codexHooksPath())
		return err == nil && strings.Contains(string(data), "LGRASS_HOOK_VENDOR=codex "+lgrass+" hook SessionStart")
	})
}
