// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/faizalv/lemongrass/internal/agentusage"
	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

// cmdUsage prints the last known rate-limit usage of every Claude account and of Codex as JSON, for the app.
func cmdUsage(args []string) {
	if len(args) == 1 && args[0] == "context" {
		cmdUsageContext()
		return
	}
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd usage [context]")
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	names, err := claudeaccount.Accounts{Home: home}.Names()
	if err != nil {
		fail(err)
	}
	accounts := append([]string{"primary"}, names...)
	report := agentusage.Collect(home, agentusage.CodexHome(home), accounts, time.Now())
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fail(err)
	}
}

// cmdUsageContext prints each Claude tab's context window use by tab id, and the directory those records live in so
// the app can watch it without knowing the layout. The directory is created here, since a watch needs it to exist.
func cmdUsageContext() {
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	dir := agentusage.ContextDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}
	tabs, err := agentusage.ReadContexts(home)
	if err != nil {
		fail(err)
	}
	out := struct {
		Dir  string                        `json:"dir"`
		Tabs map[string]agentusage.Context `json:"tabs"`
	}{dir, tabs}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fail(err)
	}
}
