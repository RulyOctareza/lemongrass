// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

func writeStatusLineStore(t *testing.T, home, body string) {
	t.Helper()
	path := agentconf.StatusLineStorePath(home)
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHumanStatusLineGetsStdinAndPassesOutput(t *testing.T) {
	home := t.TempDir()
	writeStatusLineStore(t, home, `{"commands":{"primary":"cat; echo; echo second"}}`)
	var out, errOut bytes.Buffer
	code := runHumanStatusLine(home, "primary", []byte(`{"a":1}`), &out, &errOut)
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
	if out.String() != "{\"a\":1}\nsecond\n" {
		t.Errorf("stdout %q", out.String())
	}
}

func TestHumanStatusLineExitCodePassesThrough(t *testing.T) {
	home := t.TempDir()
	writeStatusLineStore(t, home, `{"commands":{"work":"echo partial; exit 3"}}`)
	var out, errOut bytes.Buffer
	if code := runHumanStatusLine(home, "work", nil, &out, &errOut); code != 3 {
		t.Errorf("code %d, want 3", code)
	}
}

func TestNoHumanStatusLinePrintsNothing(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runHumanStatusLine(t.TempDir(), "primary", []byte(`{}`), &out, &errOut); code != 0 || out.Len() != 0 {
		t.Errorf("code %d, stdout %q", code, out.String())
	}
}
