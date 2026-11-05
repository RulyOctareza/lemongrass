// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

func TestUninstallReversesTheKeeperAndKeepsTheHumansConfig(t *testing.T) {
	k, lgrass := fakeHome(t)
	settings := k.claudeSettingsPath()
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(`{"model":"opus","statusLine":{"type":"command","command":"line.sh"},`+
		`"hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify.sh"}]}]}}`), 0o644)
	accounts := k.accounts()
	os.WriteFile(accounts.FilePath(), []byte(`{"accounts":["work"]}`), 0o600)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	if !exists(k.codexHooksPath()) || !exists(leaderSkillPath(k, codex)) {
		t.Fatal("reconcile did not set up the codex side")
	}

	bin := filepath.Join(k.home, ".local", "bin")
	os.MkdirAll(bin, 0o755)
	for _, name := range installedBinaries {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755)
	}
	serviceRemoved := false
	var out bytes.Buffer
	err := k.uninstall(&out, func(string) error { serviceRemoved = true; return nil }, []string{bin})
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !serviceRemoved {
		t.Error("service not removed")
	}

	data, _ := os.ReadFile(settings)
	for _, want := range []string{`"opus"`, "line.sh", "notify.sh"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("primary settings lost %s: %s", want, data)
		}
	}
	if strings.Contains(string(data), lgrass) {
		t.Errorf("primary settings still name lgrassd: %s", data)
	}
	work, _ := os.ReadFile(accounts.SettingsPath("work"))
	if strings.Contains(string(work), lgrass) {
		t.Errorf("account settings still name lgrassd: %s", work)
	}
	if exists(k.codexHooksPath()) {
		t.Error("codex hooks.json left behind with nothing of the human's in it")
	}
	if exists(filepath.Dir(leaderSkillPath(k, codex))) {
		t.Error("codex skill left behind")
	}
	for _, path := range []string{lgrass, filepath.Join(bin, "lgrass"), filepath.Join(bin, "lgrassconf")} {
		if exists(path) {
			t.Errorf("%s left behind", path)
		}
	}
	if got, _ := accounts.StatusLines().Command("primary"); got != "" {
		t.Errorf("saved status line kept after restoring it: %q", got)
	}
	if !exists(accounts.FilePath()) {
		t.Error("the account list was removed")
	}
}

func TestUninstallKeepsGoingAfterAFailedStep(t *testing.T) {
	k, lgrass := fakeHome(t)
	var out bytes.Buffer
	err := k.uninstall(&out, func(string) error { return errors.New("no systemd") }, nil)
	if err == nil || !strings.Contains(out.String(), "failed: removing the keeper service") {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
	if exists(lgrass) {
		t.Error("later steps did not run after the service step failed")
	}
}

func TestUninstallKeepsACodexHooksFileWithTheHumansHooks(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(k.codexHooksPath())
	mixed := strings.Replace(string(data), `"hooks": {`, `"hooks": {"Custom": [{"hooks": [{"type": "command", "command": "mine.sh"}]}],`, 1)
	os.WriteFile(k.codexHooksPath(), []byte(mixed), 0o644)

	var out bytes.Buffer
	if err := k.uninstall(&out, func(string) error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(k.codexHooksPath())
	if err != nil || !strings.Contains(string(data), "mine.sh") {
		t.Fatalf("human hook lost: %v %s", err, data)
	}
	if _, _, empty, _ := agentconf.RemoveHookGroups(data, codexHookEvents); empty {
		t.Error("file reported empty with the human's hook in it")
	}
}
