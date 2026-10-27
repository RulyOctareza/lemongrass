// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/bibliothek"
)

func embeddedOrientation(t *testing.T, name string) string {
	t.Helper()
	data, err := orientationFS.ReadFile("orientation/" + name)
	if err != nil {
		t.Fatal(err)
	}
	convention, err := bibliothek.Prompt()
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + bibliothekHeading + string(convention)
}

func TestOrientationCarriesBibliothekWithoutTheTrigger(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tab.md", "thinker.md"} {
		data, err := os.ReadFile(filepath.Join(k.orientationDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "# The bibliothek convention") || !strings.Contains(text, "## Tier 2") {
			t.Errorf("%s does not carry bibliothek", name)
		}
		if strings.Contains(text, "MANDATORY: invoke bibliothek") || strings.Contains(text, "name: bibliothek") {
			t.Errorf("%s carries the autoload trigger or the skill frontmatter", name)
		}
	}
}

func TestOrientationIsInstalledForTabsAndThinkers(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tab.md", "thinker.md"} {
		got, err := os.ReadFile(filepath.Join(k.orientationDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != embeddedOrientation(t, name) {
			t.Errorf("%s differs from its embedded copy", name)
		}
	}
}

func TestMatchingOrientationIsNotRewritten(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(k.orientationDir(), "tab.md")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Error("an unchanged orientation was rewritten")
	}
}

func TestWatcherRestoresEditedOrientation(t *testing.T) {
	k, _ := fakeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	path := filepath.Join(k.orientationDir(), "thinker.md")
	want := embeddedOrientation(t, "thinker.md")
	restored := func() bool {
		data, err := os.ReadFile(path)
		return err == nil && string(data) == want
	}
	waitFor(t, "initial install", restored)
	if err := os.WriteFile(path, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "orientation restored after an edit", restored)
	if err := os.RemoveAll(k.orientationDir()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "orientation restored after deletion", restored)
}
