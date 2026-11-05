// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/faizalv/lemongrass/internal/config"
)

func usage() {
	fmt.Fprint(os.Stderr, `lgrassconf: keeps Lemongrass agent configuration in the state it needs

  run      Watches the config and repairs drift until stopped
  check    Runs one repair pass and exits
  install  Installs the background keeper (systemd user unit on Linux, LaunchAgent on macOS, a logon task and
           the PATH entry on Windows)
  uninstall  Removes what Lemongrass put on this account: the keeper, its hooks and status line in the
             Claude and Codex config, its skills, and the lgrass, lgrassconf and lgrassd binaries.
             Lemongrass data, Claude account folders and project trust stay.
`)
	os.Exit(2)
}

func main() {
	attachParentConsole()
	if len(os.Args) < 2 {
		usage()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, err := keeperInstance(ctx, home)
		if errors.Is(err, errKeeperRunning) {
			fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
			os.Exit(1)
		}
		if err := newKeeper(home).run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
			os.Exit(1)
		}
	case "check":
		if err := newKeeper(home).reconcile(); err != nil {
			fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := installUnit(home); err != nil {
			fmt.Fprintf(os.Stderr, "lgrassconf: %v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		// Each failure is already printed by uninstall as it happens.
		if err := newKeeper(home).uninstall(os.Stdout, uninstallUnit, config.BinDirs(home)); err != nil {
			os.Exit(1)
		}
	default:
		usage()
	}
}
