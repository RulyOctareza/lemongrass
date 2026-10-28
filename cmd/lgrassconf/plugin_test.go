// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func embeddedPlugin(t *testing.T, rel string) string {
	t.Helper()
	data, err := pluginFS.ReadFile("plugin/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var pluginFiles = []string{
	"lemongrass-tools/.claude-plugin/plugin.json",
	"lemongrass-tools/hooks/hooks.json",
	"lemongrass-tools/hooks/register.ts",
}

func TestPluginIsInstalled(t *testing.T) {
	k, _ := fakeHome(t)
	if err := k.reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range pluginFiles {
		got, err := os.ReadFile(filepath.Join(k.pluginDir(), filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != embeddedPlugin(t, rel) {
			t.Errorf("%s differs from its embedded copy", rel)
		}
	}
}

func TestWatcherRestoresEditedPlugin(t *testing.T) {
	k, _ := fakeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	rel := "lemongrass-tools/hooks/register.ts"
	path := filepath.Join(k.pluginDir(), filepath.FromSlash(rel))
	want := embeddedPlugin(t, rel)
	restored := func() bool {
		data, err := os.ReadFile(path)
		return err == nil && string(data) == want
	}
	waitFor(t, "initial install", restored)
	if err := os.WriteFile(path, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "plugin restored after an edit", restored)
	if err := os.RemoveAll(k.pluginDir()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "plugin restored after deletion", restored)
}
