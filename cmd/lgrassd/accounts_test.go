// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

func TestAccountsCommandListAddRemove(t *testing.T) {
	home := t.TempDir()
	accounts := claudeaccount.Accounts{Home: home, HookBinary: "/x/lgrassd"}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := runAccounts(accounts, args, &out)
		return strings.TrimSpace(out.String()), err
	}

	if out, err := run("list"); err != nil || out != "[]" {
		t.Fatalf("empty list = %q, %v", out, err)
	}
	if out, err := run("add", "work"); err != nil || out != `{"name":"work","dir":"~/.claude-work"}` {
		t.Fatalf("add = %q, %v", out, err)
	}
	settings, err := os.ReadFile(filepath.Join(home, ".claude-work", "settings.json"))
	if err != nil || !strings.Contains(string(settings), "/x/lgrassd hook PreToolUse") {
		t.Errorf("hooks not registered with the lgrassd path: %s %v", settings, err)
	}
	if out, err := run("list"); err != nil || out != `[{"name":"work","dir":"~/.claude-work"}]` {
		t.Fatalf("list = %q, %v", out, err)
	}
	if _, err := run("add", "work"); err == nil {
		t.Error("a duplicate must fail")
	}
	if _, err := run("remove", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude-work")); !os.IsNotExist(err) {
		t.Errorf("account dir still there: %v", err)
	}
}

func TestAccountsCommandRejectsBadArguments(t *testing.T) {
	accounts := claudeaccount.Accounts{Home: t.TempDir()}
	for _, args := range [][]string{nil, {"list", "x"}, {"add"}, {"add", "a", "b"}, {"remove"}, {"nope"}} {
		if err := runAccounts(accounts, args, &bytes.Buffer{}); !errors.Is(err, errAccountsUsage) {
			t.Errorf("args %v: err = %v, want usage", args, err)
		}
	}
}
