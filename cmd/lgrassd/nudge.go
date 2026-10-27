// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"
	"os"
)

// Prints the tab's pending nudge and marks its rows sent, or prints nothing when none is pending. The app runs it to get the line it types into the tab.
func cmdNudge(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd nudge <tab-id>")
		os.Exit(1)
	}
	store := openStore()
	defer store.Close()
	if text, ok := store.ConsumePending(args[0]); ok {
		fmt.Println(text)
	}
}
