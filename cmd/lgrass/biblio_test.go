package main

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/bibliohistory"
)

func TestFormatBiblioEntry(t *testing.T) {
	at := time.Date(2026, 10, 20, 14, 3, 0, 0, time.Local)
	e := bibliohistory.Entry{
		Hash:     "a1b2c3d",
		Time:     at,
		Sessions: []string{"4fc66de4-8232-48da-bf15-623efa9f1d92", "short"},
		Changes: []bibliohistory.Change{
			{Status: "M", Path: "books/toc.md"},
			{Status: "R", OldPath: "scratchpad/t/prd.md", Path: "scratchpad/archive/t/prd.md"},
		},
	}
	want := "a1b2c3d  2026-10-20 14:03  4fc66de4,short  M books/toc.md, R scratchpad/t/prd.md -> scratchpad/archive/t/prd.md"
	if got := formatBiblioEntry(e); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}

	e.Sessions = nil
	e.Changes = nil
	for i := 0; i < 7; i++ {
		e.Changes = append(e.Changes, bibliohistory.Change{Status: "A", Path: "f"})
	}
	got := formatBiblioEntry(e)
	if !strings.Contains(got, "  -  ") || !strings.HasSuffix(got, "A f, and 2 more") {
		t.Fatalf("got %q", got)
	}
}
