// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/guard"
)

func bashEvent(t *testing.T, tab, command string) hookEvent {
	t.Helper()
	input, _ := json.Marshal(shellToolInput{Command: command})
	return hookEvent{ToolName: "Bash", ToolInput: input, Cwd: t.TempDir(), TabID: tab}
}

func verdictFor(t *testing.T, payload hookEvent, command string) guard.Verdict {
	t.Helper()
	v := guard.Decide(guard.Input{Tool: "Bash", Command: command, Cwd: payload.Cwd, Home: t.TempDir()}, guard.Policy{})
	if v == nil {
		t.Fatalf("no verdict for %q", command)
	}
	return *v
}

func TestAskModeRefusalFilesOneAsk(t *testing.T) {
	client := startVault(t)
	if err := client.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	payload := bashEvent(t, "tab-1", "git commit -m wip")
	verdict := verdictFor(t, payload, "git commit -m wip")
	if verdict.Mode == guard.ModeDeny {
		t.Fatalf("git commit is expected in ask mode, got %+v", verdict)
	}

	msg := dangerMessageWith(client.SocketPath, payload, verdict)
	if !strings.Contains(msg, "sent the user a request") || !strings.Contains(msg, "lgrass how ask request") {
		t.Fatalf("message = %q", msg)
	}
	asks, err := client.ListAsks("pw")
	if err != nil || len(asks) != 1 {
		t.Fatalf("ListAsks = %+v, %v", asks, err)
	}
	a := asks[0]
	if a.Command != "git commit -m wip" || a.Rule != verdict.RuleID || a.TabID != "tab-1" || a.Cwd != payload.Cwd {
		t.Fatalf("filed ask = %+v", a)
	}
	if !strings.Contains(msg, "ask "+a.ID) || !strings.Contains(msg, "lgrass ask show "+a.ID) {
		t.Fatalf("message %q does not name ask %s", msg, a.ID)
	}

	again := dangerMessageWith(client.SocketPath, payload, verdict)
	if again != msg {
		t.Fatalf("a repeated call filed differently: %q", again)
	}
	if asks, _ := client.ListAsks("pw"); len(asks) != 1 {
		t.Fatalf("a repeated call filed a second ask: %d asks", len(asks))
	}
}

func TestDenyModeAndFileToolsKeepTheGuardMessage(t *testing.T) {
	client := startVault(t)
	payload := bashEvent(t, "tab-1", "rm -rf ~/x")
	deny := guard.Verdict{RuleID: "rm-force", Description: "rm with a force or recursive flag", Mode: guard.ModeDeny}
	if got := dangerMessageWith(client.SocketPath, payload, deny); got != deny.Message() {
		t.Fatalf("deny message = %q, want the guard's", got)
	}

	askVerdict := verdictFor(t, payload, "git commit -m wip")
	file := hookEvent{ToolName: "Read", ToolInput: json.RawMessage(`{"file_path":"x"}`), Cwd: payload.Cwd, TabID: "tab-1"}
	if got := dangerMessageWith(client.SocketPath, file, askVerdict); got != askVerdict.Message() {
		t.Fatalf("file tool message = %q, want the guard's", got)
	}
}

func TestCompoundAskModeCallIsNotFiled(t *testing.T) {
	client := startVault(t)
	if err := client.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	command := "go vet ./... && git commit -m wip"
	payload := bashEvent(t, "tab-1", command)
	verdict := verdictFor(t, payload, command)
	if verdict.Mode == guard.ModeDeny {
		t.Fatalf("expected an ask-mode verdict, got %+v", verdict)
	}
	msg := dangerMessageWith(client.SocketPath, payload, verdict)
	if !strings.Contains(msg, "no request was sent") || !strings.Contains(msg, "Run the guarded command alone") || !strings.Contains(msg, "several commands chained") {
		t.Fatalf("message = %q", msg)
	}
	if asks, err := client.ListAsks("pw"); err != nil || len(asks) != 0 {
		t.Fatalf("a compound call was filed: %+v, %v", asks, err)
	}
}

func TestAskFilingFailureTellsTheModelToAsk(t *testing.T) {
	payload := bashEvent(t, "tab-1", "git commit -m wip")
	verdict := verdictFor(t, payload, "git commit -m wip")
	msg := dangerMessageWith(filepath.Join(t.TempDir(), "absent.sock"), payload, verdict)
	if !strings.Contains(msg, "sending the request failed") || !strings.Contains(msg, "lgrass ask") {
		t.Fatalf("message = %q", msg)
	}
}
