// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import "testing"

func TestAskAnsweredRowNudgesTheTab(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabB, "claude")

	if ok, err := store.InsertAskAnswered("no-such-tab", "abcd2345"); ok || err != nil {
		t.Fatalf("an unknown tab = %v, %v, want false with no error", ok, err)
	}
	if _, err := store.InsertAskAnswered(tabB, "Bad Id; rm"); err == nil {
		t.Fatal("took an ask id that is not letters and digits")
	}
	ok, err := store.InsertAskAnswered(tabB, "abcd2345")
	if !ok || err != nil {
		t.Fatalf("InsertAskAnswered = %v, %v", ok, err)
	}
	text, sent := store.ConsumePending(tabB)
	if !sent || text != "[lg] ask abcd2345 has its answer. Read it with lgrass ask show abcd2345." {
		t.Fatalf("ConsumePending = %q, %v", text, sent)
	}
	if again, sent := store.ConsumePending(tabB); sent {
		t.Errorf("the ask row was typed twice: %q", again)
	}
}
