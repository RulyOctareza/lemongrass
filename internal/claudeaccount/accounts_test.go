// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package claudeaccount

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testHookBinary = "/home/u/.lemongrass/bin/lgrassd"

func testAccounts(t *testing.T) Accounts {
	t.Helper()
	return Accounts{Home: t.TempDir(), HookBinary: testHookBinary}
}

func primaryWith(t *testing.T, a Accounts, entries ...string) {
	t.Helper()
	for _, entry := range entries {
		if err := os.MkdirAll(filepath.Join(a.PrimaryDir(), entry), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAddAccountLinksSharedFoldersAndRegistersHooks(t *testing.T) {
	a := testAccounts(t)
	primaryWith(t, a, "projects", "skills", "plugins")
	if err := os.WriteFile(filepath.Join(a.PrimaryDir(), "projects", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	account, err := a.Add("work")
	if err != nil {
		t.Fatal(err)
	}
	if account.Dir != "~/.claude-work" {
		t.Errorf("dir = %q", account.Dir)
	}
	for _, entry := range []string{"projects", "skills", "plugins"} {
		target, err := os.Readlink(filepath.Join(a.Dir("work"), entry))
		if err != nil || target != filepath.Join(a.PrimaryDir(), entry) {
			t.Errorf("%s link = %q, %v", entry, target, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(a.Dir("work"), "plans")); !os.IsNotExist(err) {
		t.Errorf("plans is absent in the primary, so it must not be linked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.Dir("work"), "projects", "keep")); err != nil {
		t.Errorf("shared projects not reachable: %v", err)
	}
	settings, err := os.ReadFile(a.SettingsPath("work"))
	if err != nil || !strings.Contains(string(settings), testHookBinary+" hook PreToolUse") {
		t.Errorf("hooks missing from account settings: %s %v", settings, err)
	}
	for _, name := range []string{".credentials.json", ".claude.json"} {
		if _, err := os.Lstat(filepath.Join(a.Dir("work"), name)); !os.IsNotExist(err) {
			t.Errorf("%s must stay per account and is not created: %v", name, err)
		}
	}
}

func TestAddAccountRejectsBadAndDuplicateNames(t *testing.T) {
	a := testAccounts(t)
	for _, name := range []string{"", "primary", "Work", "a b", "../x", strings.Repeat("a", 33)} {
		if _, err := a.Add(name); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Add("work"); err == nil {
		t.Error("duplicate accepted")
	}
}

func TestAddAccountRefusesAnExistingDirectory(t *testing.T) {
	a := testAccounts(t)
	if err := os.MkdirAll(a.Dir("work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Add("work"); err == nil {
		t.Fatal("an existing directory the keeper did not create must not be adopted")
	}
	names, _ := a.Names()
	if len(names) != 0 {
		t.Errorf("names = %v", names)
	}
}

func TestRemoveAccountDeletesOnlyTheAccountDirectory(t *testing.T) {
	a := testAccounts(t)
	primaryWith(t, a, "projects")
	keep := filepath.Join(a.PrimaryDir(), "projects", "keep")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Dir("work"), ".credentials.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.Remove("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(a.Dir("work")); !os.IsNotExist(err) {
		t.Errorf("account dir still there: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("shared data in the primary was touched: %v", err)
	}
	if names, _ := a.Names(); len(names) != 0 {
		t.Errorf("names = %v", names)
	}
	if err := a.Remove("work"); err == nil {
		t.Error("removing an unknown account must fail")
	}
}

func TestReconcileLinksAFolderThatAppearsLater(t *testing.T) {
	a := testAccounts(t)
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	primaryWith(t, a, "plans")
	if err := a.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(a.Dir("work"), "plans")); err != nil {
		t.Errorf("plans not linked after it appeared: %v", err)
	}
}

func TestReconcileReplacesAnEmptyDirectoryButNeverOneWithData(t *testing.T) {
	a := testAccounts(t)
	primaryWith(t, a, "plans", "projects")
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	dir := a.Dir("work")
	for _, entry := range []string{"plans", "projects"} {
		if err := os.Remove(filepath.Join(dir, entry)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, entry), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	own := filepath.Join(dir, "projects", "own")
	if err := os.WriteFile(own, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.Reconcile(); err == nil {
		t.Error("a directory with data of its own must be reported")
	}
	if _, err := os.Readlink(filepath.Join(dir, "plans")); err != nil {
		t.Errorf("empty plans not replaced by a link: %v", err)
	}
	if _, err := os.Stat(own); err != nil {
		t.Errorf("data in the account's own projects was lost: %v", err)
	}
}

func TestAccountSettingsKeepOtherKeysAndGetHooks(t *testing.T) {
	a := testAccounts(t)
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.SettingsPath("work"), []byte(`{"model":"opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(a.SettingsPath("work"))
	if !strings.Contains(string(got), `"model"`) || !strings.Contains(string(got), "hook Stop") {
		t.Errorf("settings = %s", got)
	}
}

func TestListAccounts(t *testing.T) {
	a := testAccounts(t)
	if got, err := a.List(); err != nil || len(got) != 0 {
		t.Fatalf("empty list = %v, %v", got, err)
	}
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	got, err := a.List()
	if err != nil || len(got) != 1 || got[0].Name != "work" || got[0].Dir != "~/.claude-work" {
		t.Errorf("list = %v, %v", got, err)
	}
}

func TestConcurrentReconcileNeverFailsOnTheSameLink(t *testing.T) {
	a := testAccounts(t)
	primaryWith(t, a, "projects", "skills", "plans")
	if _, err := a.Add("work"); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"projects", "skills", "plans"} {
		if err := os.Remove(filepath.Join(a.Dir("work"), entry)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.ReconcileOne("work"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent reconcile failed: %v", err)
	}
}
