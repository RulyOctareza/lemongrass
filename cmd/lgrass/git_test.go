// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"encoding/json"
	"github.com/faizalv/lemongrass/internal/ipc"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	gitTabA  = "aaaaaaaa-1111-4111-8111-111111111111"
	gitTabB  = "bbbbbbbb-2222-4222-8222-222222222222"
	gitScope = "conflict:merge:main:feature"
)

// conflictRepo is a repo stopped in a merge of feature into main with a.txt in conflict and b.txt merged cleanly.
func conflictRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
		cmd.Run()
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("a.txt", "one\nrate\nthree\n")
	write("b.txt", "b\n")
	run("add", ".")
	run("commit", "-q", "-m", "base")
	run("checkout", "-q", "-b", "feature")
	write("a.txt", "one\nTAX\nthree\n")
	write("b.txt", "b feature\n")
	run("commit", "-q", "-am", "rename")
	run("checkout", "-q", "main")
	write("a.txt", "one\nTAX_RATE\nthree\n")
	run("commit", "-q", "-am", "main side")
	run("merge", "-q", "feature")
	out, _ := exec.Command("git", "-C", dir, "diff", "--name-only", "--diff-filter=U").Output()
	if strings.TrimSpace(string(out)) != "a.txt" {
		t.Fatalf("setup: unmerged = %q, want a.txt", out)
	}
	return dir
}

type appReply func(req map[string]any) map[string]any

// fakeConflictApp stands in for the app on app.sock, records each request and answers with reply.
func fakeConflictApp(t *testing.T, reply appReply) (string, *[]map[string]any) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "app.sock")
	l, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	var got []map[string]any
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			var req map[string]any
			json.NewDecoder(conn).Decode(&req)
			got = append(got, req)
			json.NewEncoder(conn).Encode(reply(req))
			conn.Close()
		}
	}()
	return sock, &got
}

func gitEnv(t *testing.T, sock, tab string) (conflictEnv, *bytes.Buffer, *int) {
	t.Helper()
	store := openTestStore(t)
	for _, id := range []string{gitTabA, gitTabB} {
		store.RegisterTab(id, "claude")
	}
	dir := conflictRepo(t)
	var out bytes.Buffer
	notified := 0
	return conflictEnv{store: store, socket: sock, projectPath: "/proj", dir: dir, tab: tab, out: &out, notify: func() { notified++ }}, &out, &notified
}

func claimReply(grant map[string]any) appReply {
	return func(req map[string]any) map[string]any {
		if req["op"] != "conflict_claim" {
			return map[string]any{"ok": false, "error": "unexpected op"}
		}
		return map[string]any{"ok": true, "conflict": grant}
	}
}

func startConflict(t *testing.T, e conflictEnv) string {
	t.Helper()
	sock, _ := fakeConflictApp(t, claimReply(map[string]any{"action": "start", "project_path": "/proj", "scope": gitScope, "tab_id": gitTabA}))
	e.socket = sock
	var out bytes.Buffer
	e.out = &out
	if err := conflictApp(e, []string{"--token", "t1", "--json"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["threadId"] == "" {
		t.Fatalf("start output %q, %v", out.String(), err)
	}
	return got["threadId"]
}

func TestAppStartPostCloseThroughClaimedTokens(t *testing.T) {
	e, _, notified := gitEnv(t, "", gitTabA)
	id := startConflict(t, e)

	var status bytes.Buffer
	e.out = &status
	conflictStatus(e, true)
	if !strings.Contains(status.String(), `"threadId":"`+id+`"`) || !strings.Contains(status.String(), `"assignee":"`+gitTabA+`"`) {
		t.Errorf("status = %s", status.String())
	}

	sock, got := fakeConflictApp(t, claimReply(map[string]any{"action": "post", "project_path": "/proj", "scope": gitScope,
		"messages": []map[string]any{{"body": "why TAX?", "anchor": map[string]any{"path": "a.txt", "startLine": 2, "endLine": 2, "quote": "TAX"}}, {"body": "overall", "anchor": nil}}}))
	e.socket = sock
	var out bytes.Buffer
	e.out = &out
	if err := conflictApp(e, []string{"--token", "t2", "--json"}); err != nil {
		t.Fatalf("post: %v", err)
	}
	if strings.TrimSpace(out.String()) != `{"ok":true}` || *notified != 1 {
		t.Errorf("post output %q, notified %d", out.String(), *notified)
	}
	if req := (*got)[0]; req["op"] != "conflict_claim" || req["token"] != "t2" {
		t.Errorf("claim request = %v", req)
	}
	msgs, _, _ := e.store.ReadThread(id, 0, 10)
	if len(msgs) != 2 || msgs[1].TabID != session.HumanAuthor || msgs[1].Anchor == nil || msgs[1].Anchor.Quote != "TAX" {
		t.Errorf("thread = %+v", msgs)
	}

	sock, _ = fakeConflictApp(t, claimReply(map[string]any{"action": "close", "project_path": "/proj", "scope": gitScope, "thread_id": id}))
	e.socket = sock
	if err := conflictApp(e, []string{"--token", "t3", "--json"}); err != nil {
		t.Fatalf("close: %v", err)
	}
	status.Reset()
	e.out = &status
	conflictStatus(e, true)
	if strings.TrimSpace(status.String()) != "null" {
		t.Errorf("status after close = %q, want null", status.String())
	}
}

func TestAppRefusesARefusedTokenAndAnotherProject(t *testing.T) {
	e, _, _ := gitEnv(t, "", gitTabA)
	sock, _ := fakeConflictApp(t, func(map[string]any) map[string]any {
		return map[string]any{"ok": false, "error": "unknown or expired token"}
	})
	e.socket = sock
	if err := conflictApp(e, []string{"--token", "bad", "--json"}); err == nil || !strings.Contains(err.Error(), "expired token") {
		t.Errorf("refused token: err = %v", err)
	}
	sock, _ = fakeConflictApp(t, claimReply(map[string]any{"action": "start", "project_path": "/elsewhere", "scope": gitScope, "tab_id": gitTabA}))
	e.socket = sock
	if err := conflictApp(e, []string{"--token", "t", "--json"}); err == nil || !strings.Contains(err.Error(), "another project") {
		t.Errorf("other project: err = %v", err)
	}
	if err := conflictApp(e, nil); err == nil {
		t.Error("app ran without a token")
	}
	if _, err := e.store.OpenConflictThread(); err == nil {
		t.Error("a refused start opened a thread")
	}
}

func TestModelCommandsRefuseAnUnpickedTabAndNoConflict(t *testing.T) {
	sock, got := fakeConflictApp(t, func(map[string]any) map[string]any { return map[string]any{"ok": true, "text": "x"} })
	e, _, _ := gitEnv(t, sock, gitTabB)
	if err := conflictShow(e); err == nil || !strings.Contains(err.Error(), "no conflict") {
		t.Errorf("no conflict: err = %v", err)
	}
	startConflict(t, e)
	e.socket = sock
	for name, run := range map[string]func() error{
		"show":   func() error { return conflictShow(e) },
		"note":   func() error { return conflictNote(e, []string{"a.txt", "2", "why"}) },
		"submit": func() error { return conflictSubmit(e) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "handed to aaaaaaaa") {
			t.Errorf("%s from another tab: err = %v", name, err)
		}
	}
	e.tab = ""
	if err := conflictShow(e); err == nil || !strings.Contains(err.Error(), "needs a lemongrass tab") {
		t.Errorf("no tab: err = %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("a refused command reached the app: %v", *got)
	}
}

func TestShowNoteAndSubmitForTheAssignee(t *testing.T) {
	sock, got := fakeConflictApp(t, func(req map[string]any) map[string]any {
		if req["op"] == "conflict_show" {
			return map[string]any{"ok": true, "text": "the merge of feature into main\n"}
		}
		return map[string]any{"ok": true}
	})
	e, out, _ := gitEnv(t, sock, gitTabA)
	id := startConflict(t, e)
	e.socket = sock

	if err := conflictShow(e); err != nil || !strings.Contains(out.String(), "the merge of feature into main") {
		t.Fatalf("show: %v, %q", err, out.String())
	}
	if req := (*got)[0]; req["op"] != "conflict_show" || req["tab_id"] != gitTabA || req["scope"] != gitScope || req["thread_id"] != id || req["project_path"] != "/proj" {
		t.Errorf("show request = %v", req)
	}

	if err := conflictSubmit(e); err == nil || !strings.Contains(err.Error(), "nothing to submit") {
		t.Errorf("submit before any change: err = %v", err)
	}

	if err := os.WriteFile(filepath.Join(e.dir, "a.txt"), []byte("one\nTAX\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := conflictNote(e, []string{"a.txt", "2", "theirs renamed the rate to TAX"}); err != nil {
		t.Fatalf("note: %v", err)
	}
	if err := conflictNote(e, []string{"b.txt", "1", "x"}); err == nil || !strings.Contains(err.Error(), "not a conflicted file") {
		t.Errorf("note on a clean file: err = %v", err)
	}
	if err := conflictNote(e, []string{"a.txt", "9", "x"}); err == nil {
		t.Error("note past the end of the file was accepted")
	}
	if err := conflictNote(e, []string{"../a.txt", "1", "x"}); err == nil {
		t.Error("note outside the repo was accepted")
	}
	if err := conflictNote(e, []string{"a.txt", "2-1", "x"}); err == nil {
		t.Error("a backwards range was accepted")
	}

	if err := conflictSubmit(e); err != nil {
		t.Fatalf("submit: %v", err)
	}
	last := (*got)[len(*got)-1]
	if last["op"] != "conflict_submit" || last["scope"] != gitScope || last["tab_id"] != gitTabA {
		t.Errorf("submit request = %v", last)
	}
	if files, _ := last["files"].([]any); len(files) != 1 || files[0] != "a.txt" {
		t.Errorf("submit files = %v, want a.txt", last["files"])
	}

	msgs, _, _ := e.store.ReadThread(id, 0, 10)
	if len(msgs) != 2 || msgs[0].Kind != session.MessageSubmit || msgs[0].Files[0] != "a.txt" || strings.Contains(msgs[0].Body, "markers") {
		t.Errorf("submit message = %+v", msgs[0])
	}
	if msgs[1].Kind != session.MessageNote || msgs[1].Anchor == nil || msgs[1].Anchor.Path != "a.txt" || msgs[1].Anchor.Quote != "TAX" {
		t.Errorf("note message = %+v", msgs[1])
	}
}

func TestSubmitNamesFilesStillHoldingMarkers(t *testing.T) {
	sock, _ := fakeConflictApp(t, func(map[string]any) map[string]any { return map[string]any{"ok": true} })
	e, _, _ := gitEnv(t, sock, gitTabA)
	startConflict(t, e)
	e.socket = sock
	path := filepath.Join(e.dir, "a.txt")
	data, _ := os.ReadFile(path)
	os.WriteFile(path, append([]byte("// touched\n"), data...), 0o644)
	if err := conflictSubmit(e); err != nil {
		t.Fatalf("submit: %v", err)
	}
	th, _ := e.store.OpenConflictThread()
	msgs, _, _ := e.store.ReadThread(th.ID, 0, 1)
	if !strings.Contains(msgs[0].Body, "Still holding conflict markers: a.txt.") {
		t.Errorf("submit body = %q", msgs[0].Body)
	}
}

func TestThreadJSONCarriesConflictFields(t *testing.T) {
	e, _, _ := gitEnv(t, "", gitTabA)
	id := startConflict(t, e)
	e.store.PostHumanMessages(gitScope, []session.HumanMessage{{Body: "why?", Anchor: &session.Anchor{Path: "a.txt", StartLine: 2, EndLine: 2, Quote: "TAX"}}})
	e.store.PostSubmit(gitTabA, "Submitted.", []string{"a.txt"})
	tabID = gitTabA
	defer func() { tabID = "" }()

	out := captureStdout(t, func() { printThreadJSON(e.store, id, threadArgs{limit: 10}) })
	var got jsonThread
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	if got.Kind != "conflict" || got.Scope != gitScope || got.Assignee != gitTabA || got.Closed || len(got.Messages) != 2 {
		t.Fatalf("thread = %+v", got)
	}
	human, submit := got.Messages[0], got.Messages[1]
	if human.Author != "human" || human.TabID != "" || human.Label != "You" || human.Kind != "message" || human.Anchor == nil || human.Anchor.StartLine != 2 || len(human.Files) != 0 {
		t.Errorf("human message = %+v", human)
	}
	if submit.Author != "tab" || submit.TabID != gitTabA || submit.Kind != "submit" || submit.Anchor != nil || len(submit.Files) != 1 {
		t.Errorf("submit message = %+v", submit)
	}
	if !strings.Contains(out, `"files":[]`) || !strings.Contains(out, `"anchor":null`) {
		t.Errorf("empty files and anchor must be [] and null: %s", out)
	}
}
