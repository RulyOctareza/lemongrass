// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

var errAccountsUsage = errors.New("usage: lgrassd accounts <list|add|remove> ...")

func cmdAccounts(args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	if err := runAccounts(claudeaccount.Accounts{Home: home, HookBinary: hookBinary(home)}, args, os.Stdout); err != nil {
		if errors.Is(err, errAccountsUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fail(err)
	}
}

// hookBinary is the installed path the keeper also registers, so the two never rewrite each other's hook command, and falls back to the running binary when nothing is installed.
func hookBinary(home string) string {
	installed := filepath.Join(home, ".lemongrass", "bin", "lgrassd")
	if info, err := os.Stat(installed); err == nil && !info.IsDir() {
		return installed
	}
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	return self
}

func runAccounts(accounts claudeaccount.Accounts, args []string, out io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		list, err := accounts.List()
		if err != nil {
			return err
		}
		return writeJSON(out, list)
	case len(args) == 2 && args[0] == "add":
		account, err := accounts.Add(args[1])
		if err != nil {
			return err
		}
		return writeJSON(out, account)
	case len(args) == 2 && args[0] == "remove":
		return accounts.Remove(args[1])
	}
	return errAccountsUsage
}

func writeJSON(out io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
