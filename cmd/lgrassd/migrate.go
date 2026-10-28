// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/session"
)

// Brings the shared session store up to date. The app runs it at boot, before it restores tabs, so no hook pays for a migration. The store holds every project, so no project is resolved.
func cmdMigrate(args []string) {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd migrate")
		os.Exit(1)
	}
	store, err := session.Open(session.DBPath(), "")
	if err != nil {
		fail(err)
	}
	store.Close()
}
