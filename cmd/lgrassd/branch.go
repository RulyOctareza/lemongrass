// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/threadsvc"
)

// The app runs this in the project directory after its git panel moved the branch. It queues a branch notice for every recently active agent tab and wakes delivery the way a thread post does.
func cmdBranch(args []string) {
	if len(args) != 4 || args[0] != "changed" {
		fmt.Fprintln(os.Stderr, "usage: lgrassd branch changed <from> <to> <none|carried|conflict>")
		os.Exit(1)
	}
	change := session.BranchChange{From: args[1], To: args[2], Carry: args[3]}
	if !session.ValidCarry(change.Carry) {
		fmt.Fprintf(os.Stderr, "lgrassd: unknown carry %q, want none, carried or conflict\n", change.Carry)
		os.Exit(1)
	}
	store := openStore()
	defer store.Close()
	tabs, err := store.InsertBranchChanged(change, time.Now())
	if err != nil {
		fail(err)
	}
	if len(tabs) == 0 {
		return
	}
	if threadsvc.Wake(threadsvc.SocketPath()) == nil {
		return
	}
	threadsvc.NewDeliverer(store).DeliverPending()
}
