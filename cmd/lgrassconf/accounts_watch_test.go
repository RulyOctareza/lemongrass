// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

func TestWatcherHealsAccountSettingsAndPicksUpANewAccount(t *testing.T) {
	k, lgrass := fakeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitFor(t, "primary registered", func() bool {
		data, err := os.ReadFile(k.claudeSettingsPath())
		return err == nil && strings.Contains(string(data), lgrass+" hook Stop")
	})
	if _, err := k.accounts().Add("work"); err != nil {
		t.Fatal(err)
	}
	hooked := func() bool {
		data, err := os.ReadFile(k.accounts().SettingsPath("work"))
		return err == nil && strings.Contains(string(data), lgrass+" hook PreToolUse") && !strings.Contains(string(data), "Write|Edit")
	}
	waitFor(t, "account registered", hooked)

	drifted := `{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"` + lgrass + ` hook PreToolUse"}]}]}}`
	if err := agentconf.WriteAtomic(k.accounts().SettingsPath("work"), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "account matcher healed after drift", hooked)
}
