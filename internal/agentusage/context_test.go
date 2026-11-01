// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const tabA = "ba25ec5b-a010-43d8-9886-726d6b485b65"

func contextPayload(input, creation, read int64) []byte {
	return []byte(`{"session_id":"s","context_window":{"context_window_size":1000000,"current_usage":{"input_tokens":` +
		itoa(input) + `,"output_tokens":22,"cache_creation_input_tokens":` + itoa(creation) +
		`,"cache_read_input_tokens":` + itoa(read) + `},"used_percentage":5}}`)
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestRecordContextSumsInputAndCache(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(1793170000, 0)
	if err := RecordContext(home, tabA, contextPayload(2, 45204, 1000), now); err != nil {
		t.Fatal(err)
	}
	got, err := ReadContexts(home)
	if err != nil {
		t.Fatal(err)
	}
	want := Context{UsedTokens: 46206, WindowTokens: 1000000, SeenAt: now.Unix()}
	if got[tabA] != want {
		t.Errorf("context = %+v, want %+v", got[tabA], want)
	}
}

func TestRecordContextSkipsPayloadWithoutUsage(t *testing.T) {
	home := t.TempDir()
	for _, p := range []string{
		`{"session_id":"s"}`,
		`{"context_window":{"context_window_size":200000,"current_usage":null}}`,
		`{"context_window":{"context_window_size":0,"current_usage":{"input_tokens":5}}}`,
	} {
		if err := RecordContext(home, tabA, []byte(p), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := ReadContexts(home)
	if len(got) != 0 {
		t.Errorf("recorded %v", got)
	}
}

func TestRecordContextRejectsUnsafeTabID(t *testing.T) {
	home := t.TempDir()
	for _, id := range []string{"", "../escape", "a/b", "tab.id"} {
		if err := RecordContext(home, id, contextPayload(1, 1, 1), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(ContextDir(home)); !os.IsNotExist(err) {
		t.Errorf("context dir was created for an unsafe tab id: %v", err)
	}
}

func TestRecordContextLeavesUnchangedFigureAlone(t *testing.T) {
	home := t.TempDir()
	first := time.Unix(1793170000, 0)
	RecordContext(home, tabA, contextPayload(1, 100, 0), first)
	RecordContext(home, tabA, contextPayload(1, 100, 0), first.Add(time.Minute))
	got, _ := ReadContexts(home)
	if got[tabA].SeenAt != first.Unix() {
		t.Errorf("unchanged figure was rewritten: seen_at %d", got[tabA].SeenAt)
	}
	RecordContext(home, tabA, contextPayload(1, 100, 50), first.Add(2*time.Minute))
	got, _ = ReadContexts(home)
	if got[tabA].UsedTokens != 151 {
		t.Errorf("changed figure not recorded: %+v", got[tabA])
	}
}

func TestRecordContextPrunesOldRecords(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(1793170000, 0)
	stale := "11111111-2222-3333-4444-555555555555"
	RecordContext(home, stale, contextPayload(1, 1, 1), now)
	old := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(ContextDir(home), stale+".json"), old, old); err != nil {
		t.Fatal(err)
	}
	RecordContext(home, tabA, contextPayload(1, 1, 1), now)
	got, _ := ReadContexts(home)
	if _, ok := got[stale]; ok {
		t.Error("a record untouched for 8 days was kept")
	}
	if _, ok := got[tabA]; !ok {
		t.Error("the fresh record is missing")
	}
}

func TestReadContextsWithoutDirectory(t *testing.T) {
	got, err := ReadContexts(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}
