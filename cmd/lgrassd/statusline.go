// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/faizalv/lemongrass/internal/agentconf"
	"github.com/faizalv/lemongrass/internal/agentusage"
	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

// cmdStatusline is Claude Code's statusLine command, put there by lgrassconf. The visible row must look exactly as it
// did before lemongrass took the slot, so the human's own command gets the same stdin and its stdout and exit code
// pass through unchanged; recording the usage never changes what is shown.
func cmdStatusline(args []string) {
	if len(args) != 2 || args[0] != "--account" {
		fmt.Fprintln(os.Stderr, "usage: lgrassd statusline --account <name>")
		os.Exit(1)
	}
	account := args[1]
	if account != "primary" {
		if err := claudeaccount.ValidName(account); err != nil {
			fail(err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}

	recorded := make(chan struct{})
	go func() {
		defer close(recorded)
		now := time.Now()
		if err := agentusage.RecordClaude(home, account, payload, now); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		// Claude Code runs this command as its own child, so the tab id is the one lemongrass gave that Claude.
		if err := agentusage.RecordContext(home, os.Getenv(tabIDEnv), payload, now); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	code := runHumanStatusLine(home, account, payload, os.Stdout, os.Stderr)
	<-recorded
	os.Exit(code)
}

func runHumanStatusLine(home, account string, payload []byte, stdout, stderr io.Writer) int {
	store := agentconf.StatusLineStore{Path: agentconf.StatusLineStorePath(home)}
	command, err := store.Command(account)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if command == "" {
		return 0
	}
	cmd, err := statusLineCommand(command)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Claude Code cancels a run that is still going when a new trigger arrives, so the child must not outlive this
	// process.
	err = runTiedToParent(cmd)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
