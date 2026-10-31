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
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd usage")
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
