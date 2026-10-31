// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

type statusLineChainState struct {
	Enabled  bool              `json:"enabled"`
	Commands map[string]string `json:"commands"`
}

// cmdStatusLineChain reads or flips the human's off switch for the chained statusline. It only writes the marker;
// lgrassconf, the one writer of Claude config, notices it and restores or retakes the statusLine slots.
func cmdStatusLineChain(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	off := agentconf.StatusLineOffPath(home)
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "off":
		if err := agentconf.WriteAtomic(off, nil, 0o600); err != nil {
			fail(err)
		}
	case len(args) == 1 && args[0] == "on":
		if err := os.Remove(off); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fail(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: lgrassd statusline-chain [on|off]")
		os.Exit(1)
	}
	commands, err := agentconf.StatusLineStore{Path: agentconf.StatusLineStorePath(home)}.Commands()
	if err != nil {
		fail(err)
	}
	state := statusLineChainState{Enabled: agentconf.StatusLineOn(home), Commands: commands}
	if err := json.NewEncoder(os.Stdout).Encode(state); err != nil {
		fail(err)
	}
}
