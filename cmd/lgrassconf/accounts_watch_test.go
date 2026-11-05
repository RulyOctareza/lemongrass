// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

func TestWatcherHealsAccountSettingsAndPicksUpANewAccount(t *testing.T) {
	k, lgrass := fakeHome(t)
	w := startWatcher(t, k)

	w.waitFor(t, "primary registered", func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), inFile(hookCmd(lgrass, "Stop")))
	})
	if _, err := k.accounts().Add("work"); err != nil {
		t.Fatal(err)
	}
	hooked := func() bool {
		data, err := os.ReadFile(k.accounts().SettingsPath("work"))
		return err == nil && strings.Contains(string(data), inFile(hookCmd(lgrass, "PreToolUse"))) && !strings.Contains(string(data), "Write|Edit")
	}
	w.waitFor(t, "account registered", hooked)

	drifted := `{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"` + inFile(lgrass+" hook PreToolUse") + `"}]}]}}`
	if err := agentconf.WriteAtomic(k.accounts().SettingsPath("work"), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	w.waitFor(t, "account matcher healed after drift", hooked)
}
