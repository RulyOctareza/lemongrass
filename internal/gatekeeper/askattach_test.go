// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// readFramesUntil reads output frames until want has appeared, counting the replay, and fails on an end frame first.
func readFramesUntil(t *testing.T, att *AskAttachment, seen *strings.Builder, want string) {
	t.Helper()
	timer := time.AfterFunc(10*time.Second, func() { att.Close() })
	defer timer.Stop()
	for !strings.Contains(seen.String(), want) {
		f, err := att.Next()
		if err != nil {
			t.Fatalf("reading frames: %v, saw %q", err, seen.String())
		}
		if f.Type == askFrameEnd {
			t.Fatalf("end (%s) before %q, saw %q", f.Reason, want, seen.String())
		}
		seen.Write(f.Data)
	}
}

func nextEnd(t *testing.T, att *AskAttachment) string {
	t.Helper()
	timer := time.AfterFunc(10*time.Second, func() { att.Close() })
	defer timer.Stop()
	for {
		f, err := att.Next()
		if err != nil {
			t.Fatalf("reading frames: %v", err)
		}
		if f.Type == askFrameEnd {
			return f.Reason
		}
	}
}

func TestIPCAttachAskInteractive(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", `printf 'Name? '; read n; echo "hi $n"`)
	if _, err := client.AttachAsk(testRootSecret, a.ID); err == nil {
		t.Fatal("attached to an ask that is not running")
	}
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AttachAsk("wrong secret", a.ID); err == nil {
		t.Fatal("attached with a wrong secret")
	}
	att, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	var seen strings.Builder
	seen.Write(att.Replay)
	readFramesUntil(t, att, &seen, "Name?")
	if err := att.Resize(30, 120); err != nil {
		t.Fatal(err)
	}
	if err := att.Input([]byte("bob\r")); err != nil {
		t.Fatal(err)
	}
	readFramesUntil(t, att, &seen, "hi bob")
	if reason := nextEnd(t, att); reason != askViewEnded {
		t.Fatalf("end reason = %q", reason)
	}
	done := waitAskState(t, client, a.ID, vault.AskAnswered)
	if done.Result.ExitCode != 0 || done.Result.OutputTail != "Name? bob\nhi bob" {
		t.Fatalf("result = %+v", done.Result)
	}
}

func TestIPCAttachAskTakeoverAndReplay(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", `echo ready; read x; echo "took:$x"`)
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err != nil {
		t.Fatal(err)
	}
	first, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	var seen strings.Builder
	seen.Write(first.Replay)
	readFramesUntil(t, first, &seen, "ready")

	second, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if !strings.Contains(string(second.Replay), "ready") {
		t.Fatalf("replay = %q", second.Replay)
	}
	if reason := nextEnd(t, first); reason != askViewReplaced {
		t.Fatalf("first end reason = %q", reason)
	}
	if err := second.Input([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}
	var more strings.Builder
	readFramesUntil(t, second, &more, "took:yes")
	waitAskState(t, client, a.ID, vault.AskAnswered)
}

func TestIPCAttachAskDetachLeavesCommandRunning(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", `read x; echo "late:$x"`)
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err != nil {
		t.Fatal(err)
	}
	att, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	att.Close()
	time.Sleep(100 * time.Millisecond)
	if view, err := client.GetAsk(testRootSecret, a.ID); err != nil || view.State != vault.AskRunning {
		t.Fatalf("after detach: %+v, %v", view, err)
	}
	again, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.Input([]byte("ok\r")); err != nil {
		t.Fatal(err)
	}
	done := waitAskState(t, client, a.ID, vault.AskAnswered)
	if !strings.HasSuffix(done.Result.OutputTail, "late:ok") {
		t.Fatalf("tail = %q", done.Result.OutputTail)
	}
}

func TestIPCAttachAskResizeReachesCommand(t *testing.T) {
	client := startAskServer(t)
	a := fileCommand(t, client, "tab-1", `stty size; read x; stty size`)
	if _, err := client.ExecuteAsk(testRootSecret, a.ID, 0); err != nil {
		t.Fatal(err)
	}
	att, err := client.AttachAsk(testRootSecret, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	var seen strings.Builder
	seen.Write(att.Replay)
	readFramesUntil(t, att, &seen, "24 100")
	if err := att.Resize(12, 87); err != nil {
		t.Fatal(err)
	}
	if err := att.Input([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	readFramesUntil(t, att, &seen, "12 87")
}

func TestIPCAskExecuteStartsAtCardSize(t *testing.T) {
	client := startAskServer(t)
	sized := fileCommand(t, client, "tab-1", "stty size")
	if _, err := client.ExecuteAskSized(testRootSecret, sized.ID, 0, 12, 71); err != nil {
		t.Fatal(err)
	}
	if done := waitAskState(t, client, sized.ID, vault.AskAnswered); done.Result.OutputTail != "12 71" {
		t.Fatalf("tail = %q, want the card's size", done.Result.OutputTail)
	}
	wild := fileCommand(t, client, "tab-1", "stty size")
	if _, err := client.ExecuteAskSized(testRootSecret, wild.ID, 0, 5000, -1); err != nil {
		t.Fatal(err)
	}
	if done := waitAskState(t, client, wild.ID, vault.AskAnswered); done.Result.OutputTail != "24 100" {
		t.Fatalf("tail = %q, want the default size", done.Result.OutputTail)
	}
}
