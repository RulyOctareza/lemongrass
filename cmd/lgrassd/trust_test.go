package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/claudeaccount"
)

func TestTrustCommandTargets(t *testing.T) {
	home := t.TempDir()
	accounts := claudeaccount.Accounts{Home: home}
	if _, err := accounts.Add("work"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "work"} {
		if err := os.WriteFile(accounts.ClaudeJSONPath(name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	codex := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(codex), 0o700); err != nil {
		t.Fatal(err)
	}
	has := func(path, text string) bool {
		data, _ := os.ReadFile(path)
		return strings.Contains(string(data), text)
	}

	if err := runTrust(accounts, codex, []string{"/p/one", "--target", "work"}); err != nil {
		t.Fatal(err)
	}
	if !has(accounts.ClaudeJSONPath("work"), "/p/one") || has(accounts.ClaudeJSONPath(""), "/p/one") || has(codex, "/p/one") {
		t.Error("a single target must write only that target")
	}
	if err := runTrust(accounts, codex, []string{"/p/two"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{accounts.ClaudeJSONPath(""), accounts.ClaudeJSONPath("work"), codex} {
		if !has(path, "/p/two") {
			t.Errorf("%s not trusted by the all-target call", path)
		}
	}
	if err := runTrust(accounts, codex, []string{"/p/one", "--target", "ghost"}); err == nil {
		t.Error("an unknown account must be refused")
	}
	for _, args := range [][]string{{"relative"}, {}, {"/p", "--target"}} {
		if err := runTrust(accounts, codex, args); !errors.Is(err, errTrustUsage) {
			t.Errorf("%v: want usage error, got %v", args, err)
		}
	}
}
