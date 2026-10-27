// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build linux

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// hardenProcess disables ptrace attachment and core dumps for this process. Best-effort: it logs rather than fails startup if the prctl call itself errors.
func hardenProcess() {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd vault: warning: PR_SET_DUMPABLE failed: %v\n", err)
	}
}
