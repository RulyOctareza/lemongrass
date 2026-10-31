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
	"runtime"
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
		if err := agentusage.RecordClaude(home, account, payload, time.Now()); err != nil {
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
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Claude Code cancels a run that is still going when a new trigger arrives, so the child must not outlive this
	// process. The parent-death signal is tied to the thread that forks, hence the lock.
	dieWithParent(cmd)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err = cmd.Run()
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
