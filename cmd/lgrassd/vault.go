// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
)

const (
	vaultAdminMaxFailures = 5
	vaultAdminWindow      = time.Minute
	vaultAdminLockout     = 5 * time.Minute
)

func cmdVault(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd vault run")
		os.Exit(1)
	}
	switch args[0] {
	case "run":
		cmdVaultRun()
	default:
		fmt.Fprintf(os.Stderr, "unknown vault command: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdVaultRun() {
	hardenProcess()

	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sockPath := gatekeeper.SocketPath()
	os.Remove(sockPath) // a stale socket left behind by a previous, uncleanly-stopped run

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer l.Close()
	if err := os.Chmod(sockPath, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	svc, err := gatekeeper.NewBackend(filepath.Join(config.Dir(), "vault"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	limiter := vault.NewFailureLimiter(vaultAdminMaxFailures, vaultAdminWindow, vaultAdminLockout)

	fmt.Printf("lgrassd vault: listening on %s\n", sockPath)
	if err := gatekeeper.Serve(svc, l, limiter); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
