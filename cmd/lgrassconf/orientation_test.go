package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func embeddedOrientation(t *testing.T, name string) string {
	t.Helper()
	data, err := orientationFS.ReadFile("orientation/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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
