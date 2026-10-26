// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseThreadArgs(t *testing.T) {
	parsed := parseThreadArgs([]string{"12", "-", "--before", "30", "--limit", "5", "--file", "x.txt"}, 10)
	if len(parsed.positional) != 2 || parsed.positional[0] != "12" || parsed.positional[1] != "-" {
		t.Errorf("positional = %q, want [12 -]", parsed.positional)
	}
	if parsed.before != 30 || parsed.limit != 5 || parsed.file != "x.txt" {
		t.Errorf("parsed = %+v, want before 30, limit 5, file x.txt", parsed)
	}
	if got := parseThreadArgs([]string{"--limit", "0"}, 10); got.limit != 10 {
		t.Errorf("limit 0 gave %d, want the default 10", got.limit)
	}
}

func TestThreadContentSources(t *testing.T) {
	if got, _ := threadContent(threadArgs{positional: []string{"12", "  hello  "}}, 1); got != "hello" {
		t.Errorf("positional content = %q, want hello", got)
	}
	path := filepath.Join(t.TempDir(), "note.txt")
	os.WriteFile(path, []byte("from a file\n"), 0o600)
	if got, _ := threadContent(threadArgs{positional: []string{"12"}, file: path}, 1); got != "from a file" {
		t.Errorf("file content = %q, want from a file", got)
	}
	if _, err := threadContent(threadArgs{file: filepath.Join(t.TempDir(), "missing")}, 1); err == nil {
		t.Error("missing file gave no error")
	}
}
