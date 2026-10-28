// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	kindsLeader = "aaaaaaaa-1111-4111-8111-111111111111"
	kindsOne    = "bbbbbbbb-2222-4222-8222-222222222222"
	kindsTwo    = "cccccccc-3333-4333-8333-333333333333"
)

func storeWithKindsGroup(t *testing.T) (*session.Store, session.Group) {
	t.Helper()
	store := openTestStore(t)
	store.RegisterTab(kindsLeader, "claude")
	g, err := store.CreateGroup("Review", session.Member{TabID: kindsLeader, Label: "lead", Vendor: "claude"}, []session.Member{
		{TabID: kindsOne, Label: "reviewer", Vendor: "claude"},
		{TabID: kindsTwo, Label: "tester", Vendor: "claude"},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return store, g
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = saved
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestPrintThreadShowsOnlyUnreadAfterTheFirstRead(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	tabID = kindsOne
	t.Cleanup(func() { tabID = "" })
	args := threadArgs{limit: defaultThreadReadLimit}

	store.PostMessage(kindsLeader, g.ThreadID, "first news")
	first := captureStdout(t, func() { printThread(store, g.ThreadID, args, nil) })
	if !strings.Contains(first, "first news") || strings.Contains(first, "unread only") {
		t.Fatalf("first read = %q, want the latest page", first)
	}

	store.PostMessage(kindsLeader, g.ThreadID, "second news")
	second := captureStdout(t, func() { printThread(store, g.ThreadID, args, nil) })
	if !strings.Contains(second, "second news") || strings.Contains(second, "first news") || !strings.Contains(second, "unread only") {
		t.Errorf("second read = %q, want only the new message", second)
	}

	third := captureStdout(t, func() { printThread(store, g.ThreadID, args, nil) })
	if !strings.Contains(third, "nothing new") {
		t.Errorf("third read = %q, want the nothing new line", third)
	}

	all := captureStdout(t, func() { printThread(store, g.ThreadID, threadArgs{limit: 10, all: true}, nil) })
	if !strings.Contains(all, "first news") || !strings.Contains(all, "second news") {
		t.Errorf("--all read = %q, want the latest page", all)
	}
}

func TestPrintThreadWithoutATabKeepsTheLatestPage(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	tabID = ""
	store.PostMessage(kindsLeader, g.ThreadID, "one")
	for i := 0; i < 2; i++ {
		out := captureStdout(t, func() { printThread(store, g.ThreadID, threadArgs{limit: 10}, nil) })
		if !strings.Contains(out, "one") || strings.Contains(out, "unread only") {
			t.Errorf("read %d = %q, want the full latest page every time", i, out)
		}
	}
}

func TestPrintThreadHeaderIsFullOnlyOnTheFirstRead(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	tabID = kindsTwo
	t.Cleanup(func() { tabID = "" })
	var firsts []bool
	header := func(first bool) string {
		firsts = append(firsts, first)
		return "header"
	}
	captureStdout(t, func() { printThread(store, g.ThreadID, threadArgs{limit: 10}, header) })
	store.PostMessage(kindsLeader, g.ThreadID, "x")
	captureStdout(t, func() { printThread(store, g.ThreadID, threadArgs{limit: 10}, header) })
	captureStdout(t, func() { printThread(store, g.ThreadID, threadArgs{limit: 10, all: true}, header) })
	if len(firsts) != 3 || !firsts[0] || firsts[1] || !firsts[2] {
		t.Errorf("first flags = %v, want true, false, true", firsts)
	}
}
