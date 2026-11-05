// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"github.com/faizalv/lemongrass/internal/testenv"
	"os"
	"path/filepath"
	"testing"

	"github.com/faizalv/lemongrass/internal/bibliohistory"
	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/project"
)

func writeRegistry(t *testing.T, projects []project.Project) {
	t.Helper()
	data, err := json.Marshal(projects)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.Dir(), "projects.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPurgeProjectRefusesALiveProject(t *testing.T) {
	testenv.Home(t)
	writeRegistry(t, []project.Project{{ID: "live", Name: "live", Path: t.TempDir()}})
	history := bibliohistory.GitDir("live")
	if err := os.MkdirAll(history, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := purgeProject("live"); err == nil {
		t.Fatal("purgeProject purged a project not marked deleted")
	}
	if _, err := os.Stat(history); err != nil {
		t.Errorf("history of a live project was touched: %v", err)
	}
	if err := purgeProject("unknown"); err == nil {
		t.Error("purgeProject accepted an unregistered id")
	}
}

func TestPurgeProjectRemovesHistoryOfADeletedProject(t *testing.T) {
	testenv.Home(t)
	dir := t.TempDir()
	writeRegistry(t, []project.Project{{ID: "gone", Name: "gone", Path: dir, DeletedAt: "2026-01-01T00:00:00.000Z"}})
	history := bibliohistory.GitDir("gone")
	if err := os.MkdirAll(history, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := purgeProject("gone"); err != nil {
		t.Fatalf("purgeProject: %v", err)
	}
	if _, err := os.Stat(history); !os.IsNotExist(err) {
		t.Errorf("history repo still present: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("project folder was touched: %v", err)
	}
}
