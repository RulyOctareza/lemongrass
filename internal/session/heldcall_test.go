// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"errors"
	"testing"
	"time"
)

func TestHeldCallRoundTrip(t *testing.T) {
	store := openTestStore(t)
	id, err := store.HoldCall(HeldCall{SessionID: "session-a", TabID: "tab", ToolName: "Bash", Payload: []byte(`{"x":1}`), HookBinary: "/bin/lgrassd"})
	if err != nil {
		t.Fatalf("HoldCall: %v", err)
	}
	got, err := store.HeldCall("session-a", id)
	if err != nil {
		t.Fatalf("HeldCall: %v", err)
	}
	if got.ToolName != "Bash" || string(got.Payload) != `{"x":1}` || got.HookBinary != "/bin/lgrassd" || got.TabID != "tab" {
		t.Errorf("HeldCall = %+v, want the stored fields back", got)
	}
	list, err := store.HeldCalls("session-a")
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Errorf("HeldCalls = %v, %v, want the one held call", list, err)
	}
}

func TestHeldCallRefusesAnotherSession(t *testing.T) {
	store := openTestStore(t)
	id, err := store.HoldCall(HeldCall{SessionID: "session-a", ToolName: "Bash", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.HeldCall("session-b", id); !errors.Is(err, ErrNoHeldCall) {
		t.Errorf("HeldCall from another session error = %v, want ErrNoHeldCall", err)
	}
	if dropped, _ := store.DropHeldCall("session-b", id); dropped {
		t.Error("DropHeldCall from another session = true, want false")
	}
}

func TestHeldCallRunsOnce(t *testing.T) {
	store := openTestStore(t)
	id, err := store.HoldCall(HeldCall{SessionID: "session-a", ToolName: "Bash", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if dropped, err := store.DropHeldCall("session-a", id); err != nil || !dropped {
		t.Fatalf("first DropHeldCall = %v, %v, want true", dropped, err)
	}
	if dropped, _ := store.DropHeldCall("session-a", id); dropped {
		t.Error("second DropHeldCall = true, want false")
	}
	if _, err := store.HeldCall("session-a", id); !errors.Is(err, ErrNoHeldCall) {
		t.Errorf("HeldCall after drop error = %v, want ErrNoHeldCall", err)
	}
}

func TestHeldCallExpires(t *testing.T) {
	store := openTestStore(t)
	id, err := store.HoldCall(HeldCall{SessionID: "session-a", ToolName: "Bash", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-HeldCallTTL - time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`UPDATE lg_held_calls SET held_at = ? WHERE id = ?`, old, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HeldCall("session-a", id); !errors.Is(err, ErrNoHeldCall) {
		t.Errorf("expired HeldCall error = %v, want ErrNoHeldCall", err)
	}
	if list, _ := store.HeldCalls("session-a"); len(list) != 0 {
		t.Errorf("HeldCalls = %v, want none after expiry", list)
	}
	if _, err := store.HoldCall(HeldCall{SessionID: "session-a", ToolName: "Bash", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM lg_held_calls WHERE id = ?`, id).Scan(&n); err != nil || n != 0 {
		t.Errorf("expired row count after a new hold = %d, %v, want 0", n, err)
	}
}
