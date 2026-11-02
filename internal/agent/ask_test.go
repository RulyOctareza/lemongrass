// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
)

// The model's ask path runs lgrass to the agent to the vault, and only the four model ops are relayed.
func TestIPCAskRelay(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	agentClient := startTestAgent(t, vaultClient)

	a, err := agentClient.CreateAsk(gatekeeper.AskRequest{Type: vault.AskRester, TabID: "tab-1", Cwd: t.TempDir(), Message: "GET on /orders", Methods: []string{"GET"}})
	if err != nil || a.ID == "" {
		t.Fatalf("CreateAsk = %+v, %v", a, err)
	}
	got := make(chan vault.Ask, 1)
	go func() {
		w, err := agentClient.WaitAsk(a.ID, "tab-1", time.Minute)
		if err != nil {
			t.Errorf("WaitAsk: %v", err)
		}
		got <- w
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, blocking, _, _ := vaultClient.AskCount(); blocking == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the relayed wait never reached the vault")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := vaultClient.DeclineAsk(testRootSecret, a.ID, "use the staging rester"); err != nil {
		t.Fatal(err)
	}
	if w := <-got; w.State != vault.AskDeclined || w.Note != "use the staging rester" {
		t.Fatalf("relayed wait = %+v", w)
	}
	if err := agentClient.AckAsk(a.ID, "tab-1"); err != nil {
		t.Fatalf("AckAsk: %v", err)
	}
	if r, err := agentClient.AskResult(a.ID, "tab-1"); err != nil || r.State != vault.AskDeclined {
		t.Fatalf("AskResult = %+v, %v", r, err)
	}

	var out struct{}
	for _, op := range []string{"list_asks", "get_ask", "answer_ask", "execute_ask", "stop_ask", "decline_ask", "ask_count"} {
		if err := agentClient.call(op, map[string]string{"root_secret": testRootSecret, "id": a.ID}, &out); err == nil {
			t.Errorf("the agent relayed the app op %s", op)
		}
	}
}
