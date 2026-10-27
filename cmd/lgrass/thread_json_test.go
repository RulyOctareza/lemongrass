// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"encoding/json"
	"testing"
)

func TestPrintThreadJSONIsOldestFirstWithLabelsAndNoReadMarks(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	tabID = kindsTwo
	store.PostMessage(kindsLeader, g.ThreadID, "first")
	store.PostMessage(kindsOne, g.ThreadID, "second")
	store.PostMessage(kindsLeader, g.ThreadID, "third")

	for i := 0; i < 2; i++ {
		out := captureStdout(t, func() { printThreadJSON(store, g.ThreadID, threadArgs{limit: 2}) })
		var got jsonThread
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("output %q is not JSON: %v", out, err)
		}
		if got.Title != "Review" || got.GroupID != g.ID || !got.More || len(got.Messages) != 2 {
			t.Fatalf("read %d = %+v, want a titled page of 2 with older ones remaining", i, got)
		}
		if got.Messages[0].Body != "second" || got.Messages[0].Label != "reviewer" || got.Messages[1].Body != "third" || got.Messages[1].Label != "lead" {
			t.Errorf("read %d messages = %+v, want oldest first with member labels", i, got.Messages)
		}
	}
	older := captureStdout(t, func() { printThreadJSON(store, g.ThreadID, threadArgs{limit: 2, before: 2}) })
	var page jsonThread
	if err := json.Unmarshal([]byte(older), &page); err != nil || len(page.Messages) != 1 || page.Messages[0].Body != "first" || page.More {
		t.Errorf("older page = %q (%v), want just the first message and nothing older", older, err)
	}
}
