// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func startAskServer(t *testing.T) *Client {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	client := startTestServer(t)
	if err := client.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	return client
}

func fileCommand(t *testing.T, client *Client, tab, command string) vault.Ask {
	t.Helper()
	a, err := client.CreateAsk(AskRequest{Type: vault.AskCommand, TabID: tab, Cwd: t.TempDir(), Command: command, Reason: "needed"})
	if err != nil {
		t.Fatalf("CreateAsk: %v", err)
	}
	return a
}

func waitAskState(t *testing.T, client *Client, id, state string) AskView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		a, err := client.GetAsk(testRootSecret, id)
		if err != nil {
			t.Fatalf("GetAsk: %v", err)
		}
		if a.State == state {
			return a
		}
		if time.Now().After(deadline) {
			t.Fatalf("ask %s stayed %s, want %s", id, a.State, state)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestIPCAskWaitDeclineAndCount(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", "git commit -m wip")

	waiting, blocking, newest, err := client.AskCount()
	if err != nil || waiting != 1 || blocking != 0 || newest != a.ID {
		t.Fatalf("AskCount = %d %d %q %v", waiting, blocking, newest, err)
	}
	if _, err := client.ListAsks("wrong secret"); err == nil {
		t.Fatal("ListAsks took a wrong secret")
	}
	if err := client.DeclineAsk("wrong secret", a.ID, "no"); err == nil {
		t.Fatal("DeclineAsk took a wrong secret")
	}

	got := make(chan vault.Ask, 1)
	go func() {
		w, err := client.WaitAsk(a.ID, "tab-1", time.Minute)
		if err != nil {
			t.Errorf("WaitAsk: %v", err)
		}
		got <- w
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, b, _, _ := client.AskCount(); b == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the waiter never showed as blocking")
		}
		time.Sleep(10 * time.Millisecond)
	}
	list, err := client.ListAsks(testRootSecret)
	if err != nil || len(list) != 1 || !list[0].Blocking {
		t.Fatalf("ListAsks = %+v, %v", list, err)
	}
	if list[0].Verdict == nil || list[0].Verdict.Rule != "git-commit" || list[0].Verdict.Mode != "ask" {
		t.Fatalf("verdict = %+v, want git-commit in ask mode", list[0].Verdict)
	}
	if err := client.DeclineAsk(testRootSecret, a.ID, "not on main"); err != nil {
		t.Fatal(err)
	}
	w := <-got
	if w.State != vault.AskDeclined || w.Note != "not on main" {
		t.Fatalf("waiter got %+v", w)
	}
	r, err := client.AskResult(a.ID, "tab-1")
	if err != nil || r.State != vault.AskDeclined {
		t.Fatalf("AskResult = %+v, %v", r, err)
	}
	if _, err := client.AskResult(a.ID, "tab-2"); err == nil {
		t.Fatal("another tab read the ask")
	}
}

func TestIPCAskExecute(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", "echo one; echo two >&2; exit 3")
	view, err := client.ExecuteAsk(testRootSecret, a.ID, 0)
	if err != nil || view.State != vault.AskRunning {
		t.Fatalf("ExecuteAsk = %+v, %v", view, err)
	}
	done := waitAskState(t, client, a.ID, vault.AskAnswered)
	if done.Result == nil || done.Result.ExitCode != 3 || done.Result.OutputTail != "one\ntwo" || done.Result.TimedOut || done.Result.Stopped {
		t.Fatalf("result = %+v", done.Result)
	}
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err == nil {
		t.Fatal("executed an answered ask again")
	}
}

func TestIPCAskExecuteTimeoutAndStop(t *testing.T) {
	client := startAskServer(t)
	slow := fileCommand(t, client, "tab-1", "sleep 30")
	if _, err := client.ExecuteAsk(testRootSecret, slow.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	timedOut := waitAskState(t, client, slow.ID, vault.AskAnswered)
	if !timedOut.Result.TimedOut {
		t.Fatalf("result = %+v, want timed out", timedOut.Result)
	}

	stop := fileCommand(t, client, "tab-1", "sleep 30")
	if err := client.StopAsk(testRootSecret, stop.ID); err == nil {
		t.Fatal("stopped an ask that was not running")
	}
	if _, err := client.ExecuteAsk(testRootSecret, stop.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := client.StopAsk(testRootSecret, stop.ID); err != nil {
		t.Fatal(err)
	}
	stopped := waitAskState(t, client, stop.ID, vault.AskAnswered)
	if !stopped.Result.Stopped || stopped.Result.EndedAt.Sub(stopped.Result.StartedAt) > 10*time.Second {
		t.Fatalf("result = %+v, want stopped promptly", stopped.Result)
	}
}

func TestIPCAskNeverExecute(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", "rm -rf /")
	view, err := client.GetAsk(testRootSecret, a.ID)
	if err != nil || !view.NeverExecute || view.NeverExecuteReason == "" {
		t.Fatalf("GetAsk = %+v, %v", view, err)
	}
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err == nil || !strings.Contains(err.Error(), "never executes") {
		t.Fatalf("ExecuteAsk = %v, want a never-execute refusal", err)
	}
	if got, _ := client.GetAsk(testRootSecret, a.ID); got.State != vault.AskWaiting {
		t.Fatalf("state after refusal = %s", got.State)
	}
}

func TestIPCAskChannel(t *testing.T) {
	client := startAskServer(t)
	cmd := fileCommand(t, client, "tab-1", "ls")
	if _, err := client.CreateChannelForAsk(testRootSecret, cmd.ID, "c", "conn", fullScope(), time.Hour); err == nil || !strings.Contains(err.Error(), "not a db ask") {
		t.Fatalf("db channel for a command ask = %v", err)
	}
	if err := client.AnswerAsk(testRootSecret, cmd.ID, "abc123"); err == nil {
		t.Fatal("answered a command ask with a channel")
	}

	if err := client.PutCredential(testRootSecret, "conn", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	db, err := client.CreateAsk(AskRequest{Type: vault.AskDB, TabID: "tab-1", Cwd: t.TempDir(), Tables: []string{"orders"}, Reason: "report"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateChannelForAsk(testRootSecret, db.ID, "c", "conn", fullScope(), time.Hour); err == nil {
		t.Fatal("created a channel without listing the connection's tables")
	}
	if err := client.AnswerAsk(testRootSecret, db.ID, "abc123"); err == nil || !strings.Contains(err.Error(), "no channel was created") {
		t.Fatalf("AnswerAsk with no channel = %v", err)
	}

	domain := vaulttest.Domain{BaseURL: "http://127.0.0.1:1", LoginEndpoint: "/login", TokenPath: "t", FixedTTLSeconds: 60,
		TokenPlacement: vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization"},
		Users:          []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}}
	if err := client.PutRester(testRootSecret, "api", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	rester, err := client.CreateAsk(AskRequest{Type: vault.AskRester, TabID: "tab-1", Cwd: t.TempDir(), Message: "GET on /orders"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateHTTPChannelForAsk(testRootSecret, rester.ID, "c", "api", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour); err != nil {
		t.Fatalf("CreateHTTPChannelForAsk: %v", err)
	}
	if err := client.AnswerAsk(testRootSecret, rester.ID, "abc123"); err != nil {
		t.Fatalf("AnswerAsk: %v", err)
	}
	r, err := client.AskResult(rester.ID, "tab-1")
	if err != nil || r.State != vault.AskAnswered || r.ShortID != "abc123" {
		t.Fatalf("AskResult = %+v, %v", r, err)
	}
	if _, err := client.CreateHTTPChannelForAsk(testRootSecret, rester.ID, "c2", "api", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour); err == nil {
		t.Fatal("created a second channel for an answered ask")
	}
}

func TestMissingTables(t *testing.T) {
	cases := []struct {
		requested, have, want []string
	}{
		{[]string{"orders"}, []string{"orders", "customers"}, nil},
		{[]string{"Orders"}, []string{"orders"}, nil},
		{[]string{"orders", "refunds"}, []string{"orders"}, []string{"refunds"}},
		{[]string{"a"}, nil, []string{"a"}},
	}
	for _, tc := range cases {
		got := missingTables(tc.requested, tc.have)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("missingTables(%v, %v) = %v, want %v", tc.requested, tc.have, got, tc.want)
		}
	}
}

func TestTailWriter(t *testing.T) {
	w := newTailWriter(3, 5)
	w.Write([]byte("a\nb\nc\nd\r\n"))
	w.Write([]byte("toolongline\npart"))
	if got := w.String(); got != "d\ntoolo [line cut]\npart" {
		t.Fatalf("tail = %q", got)
	}
}

func TestAskEnv(t *testing.T) {
	env := askEnv([]string{"PATH=/daemon", "HOME=/h", "LGRASS_TAB_ID=x"}, "/tab/bin")
	if strings.Join(env, " ") != "HOME=/h PATH=/tab/bin" {
		t.Fatalf("askEnv = %v", env)
	}
	if env := askEnv([]string{"PATH=/daemon"}, ""); env[0] != "PATH=/daemon" {
		t.Fatalf("askEnv with no caller PATH = %v", env)
	}
}
