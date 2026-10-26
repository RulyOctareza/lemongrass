// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func storeVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	return v
}

func TestOpenBuildsAFreshStoreAtTheCurrentVersion(t *testing.T) {
	store := openTestStore(t)
	if v := storeVersion(t, store); v != len(migrations) {
		t.Errorf("version = %d, want %d", v, len(migrations))
	}
	if err := store.RegisterTab("aaaaaaaa-0000", "claude"); err != nil {
		t.Errorf("RegisterTab on a fresh store: %v", err)
	}
}

// A store from before versioning reads 0 and keeps its rows through version 1.
func TestOpenMigratesAnUnversionedStoreKeepingItsData(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(DBPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	old, err := sql.Open("sqlite", DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := old.Exec(schema + `CREATE TABLE lg_tabs (project_id TEXT NOT NULL, tab_id TEXT NOT NULL, vendor TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (project_id, tab_id));
		INSERT INTO lg_tips (project_id, message, created_at) VALUES ('testproj', 'keep me', '2026-01-01T00:00:00Z');
		INSERT INTO lg_tabs VALUES ('testproj', 'old-tab', 'codex', '2026-01-01T00:00:00Z');`); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	old.Close()

	store, err := Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if v := storeVersion(t, store); v != len(migrations) {
		t.Errorf("version = %d, want %d", v, len(migrations))
	}
	tips, err := store.ListTips()
	if err != nil || len(tips) != 1 || tips[0].Message != "keep me" {
		t.Errorf("tips = %+v, %v", tips, err)
	}
	if vendor, _ := store.VendorForTab("old-tab"); vendor != "codex" {
		t.Errorf("vendor = %q, want codex", vendor)
	}
	var title string
	if err := store.db.QueryRow(`SELECT title FROM lg_tabs WHERE tab_id = 'old-tab'`).Scan(&title); err != nil {
		t.Errorf("title column after migration: %v", err)
	}
}

// A store as version 1 left it, for tests that seed pre-versioning tables and reset user_version to 0.
func openVersion1Store(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	saved := migrations
	migrations = migrations[:1]
	store, err := Open(DBPath(), testProjectID)
	migrations = saved
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func withMigrations(t *testing.T, steps []func(execer) error) {
	t.Helper()
	saved := migrations
	migrations = steps
	t.Cleanup(func() { migrations = saved })
}

func TestOpenOnACurrentStoreRunsNoStep(t *testing.T) {
	openTestStore(t)
	var runs int32
	steps := append([]func(execer) error{}, migrations...)
	for i := range steps {
		steps[i] = func(execer) error { atomic.AddInt32(&runs, 1); return nil }
	}
	withMigrations(t, steps)

	reopened, err := Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reopened.Close()
	if runs != 0 {
		t.Errorf("%d steps ran on a current store, want none", runs)
	}
}

// The race that matters is the app's boot migrate against a hook from a tab left running, on a store that already exists in WAL mode.
func TestConcurrentOpensApplyAStepOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(DBPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	old, err := sql.Open("sqlite", DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := old.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	old.Close()
	var runs int32
	withMigrations(t, []func(execer) error{func(db execer) error {
		atomic.AddInt32(&runs, 1)
		time.Sleep(50 * time.Millisecond)
		_, err := db.ExecContext(context.Background(), `CREATE TABLE once (id INTEGER)`)
		return err
	}})

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := Open(DBPath(), testProjectID)
			if err == nil {
				s.Close()
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("open %d: %v", i, err)
		}
	}
	if runs != 1 {
		t.Errorf("step ran %d times, want 1", runs)
	}
}

func TestOpenRefusesANewerStore(t *testing.T) {
	store := openTestStore(t)
	newer := len(migrations) + 1
	if _, err := store.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, newer)); err != nil {
		t.Fatalf("setting version: %v", err)
	}
	_, err := Open(DBPath(), testProjectID)
	if err == nil {
		t.Fatal("Open accepted a newer store")
	}
	if !strings.Contains(err.Error(), fmt.Sprint(newer)) || !strings.Contains(err.Error(), fmt.Sprint(len(migrations))) {
		t.Errorf("error %q does not name both versions", err)
	}
}
