// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"strings"
	"sync"
	"testing"
	"time"
)

type askClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *askClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *askClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type askRecorder struct {
	mu          sync.Mutex
	changes     int
	undelivered []Ask
}

func newTestAskStore() (*AskStore, *askClock, *askRecorder) {
	s := NewAskStore()
	clock := &askClock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	s.now = clock.now
	s.grace = 30 * time.Millisecond
	rec := &askRecorder{}
	s.OnChange = func() { rec.mu.Lock(); rec.changes++; rec.mu.Unlock() }
	s.OnUndelivered = func(a Ask) { rec.mu.Lock(); rec.undelivered = append(rec.undelivered, a); rec.mu.Unlock() }
	return s, clock, rec
}

func (r *askRecorder) undeliveredIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for _, a := range r.undelivered {
		ids = append(ids, a.ID)
	}
	return ids
}

func commandAsk(tab, command string) AskInput {
	return AskInput{Type: AskCommand, TabID: tab, Cwd: "/work/app", Command: command, Reason: "needed now"}
}

func TestAskCreateValidation(t *testing.T) {
	cases := []struct {
		name string
		in   AskInput
		err  string
	}{
		{"db ok", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"orders"}, Reason: "r"}, ""},
		{"rester ok", AskInput{Type: AskRester, TabID: "t", Cwd: "/w", Message: "I need GET on /x"}, ""},
		{"command ok", commandAsk("t", "git push"), ""},
		{"hook command needs no reason", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Command: "git push", Rule: "git-push"}, ""},
		{"unknown type", AskInput{Type: "x", TabID: "t", Cwd: "/w"}, "unknown ask type"},
		{"no tab", AskInput{Type: AskRester, Cwd: "/w", Message: "m"}, "tab id"},
		{"relative cwd", AskInput{Type: AskRester, TabID: "t", Cwd: "w", Message: "m"}, "absolute"},
		{"db no tables", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Reason: "r"}, "at least one table"},
		{"db no reason", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"a"}}, "needs a reason"},
		{"db bad table", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"a b"}, Reason: "r"}, "not a table name"},
		{"db too many tables", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: make([]string, MaxAskTables+1), Reason: "r"}, "at most"},
		{"db with command", AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"a"}, Reason: "r", Command: "x"}, "tables and a reason only"},
		{"rester empty", AskInput{Type: AskRester, TabID: "t", Cwd: "/w"}, "needs a message"},
		{"rester long", AskInput{Type: AskRester, TabID: "t", Cwd: "/w", Message: strings.Repeat("m", MaxAskMessage+1)}, "cap"},
		{"command empty", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Reason: "r"}, "needs a command"},
		{"command long", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Reason: "r", Command: strings.Repeat("c", MaxAskCommand+1)}, "cap"},
		{"command no reason", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Command: "ls"}, "needs a reason"},
		{"reason long", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Command: "ls", Reason: strings.Repeat("r", MaxAskReason+1)}, "cap"},
		{"invalid utf8", AskInput{Type: AskCommand, TabID: "t", Cwd: "/w", Command: "ls", Reason: "\xff"}, "UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestAskStore()
			a, err := s.Create(tc.in)
			if tc.err == "" {
				if err != nil {
					t.Fatalf("Create: %v", err)
				}
				if a.ID == "" || a.State != AskWaiting {
					t.Fatalf("Create returned %+v", a)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("Create error = %v, want %q", err, tc.err)
			}
		})
	}
}

func TestAskHookDedupeAndLimits(t *testing.T) {
	s, clock, _ := newTestAskStore()
	hook := AskInput{Type: AskCommand, TabID: "t1", Cwd: "/w", Command: "git push", Rule: "git-push"}
	first, err := s.Create(hook)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Create(hook)
	if err != nil || again.ID != first.ID {
		t.Fatalf("a repeated hook ask = %v, %v, want the first ask %s", again.ID, err, first.ID)
	}
	if other, _ := s.Create(AskInput{Type: AskCommand, TabID: "t2", Cwd: "/w", Command: "git push", Rule: "git-push"}); other.ID == first.ID {
		t.Fatal("another tab's hook ask was folded into the first tab's")
	}

	for i := 0; i < askOpenMax-1; i++ {
		if _, err := s.Create(commandAsk("t1", "ls "+string(rune('a'+i)))); err != nil {
			t.Fatalf("open ask %d: %v", i, err)
		}
	}
	if _, err := s.Create(commandAsk("t1", "ls z")); err == nil || !strings.Contains(err.Error(), "asks open") {
		t.Fatalf("open cap error = %v", err)
	}

	s2, _, _ := newTestAskStore()
	s2.now = clock.now
	for i := 0; i < askRateMax; i++ {
		a, err := s2.Create(commandAsk("t", "ls"))
		if err != nil {
			t.Fatalf("filing %d: %v", i, err)
		}
		if err := s2.Decline(a.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s2.Create(commandAsk("t", "ls")); err == nil || !strings.Contains(err.Error(), "wait before asking") {
		t.Fatalf("rate limit error = %v", err)
	}
	clock.advance(askRateWindow)
	if _, err := s2.Create(commandAsk("t", "ls")); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}

func TestAskWaitAnswered(t *testing.T) {
	s, _, rec := newTestAskStore()
	a, _ := s.Create(AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"orders"}, Reason: "r"})
	if err := s.AnswerChannel(a.ID, "ab12cd"); err == nil {
		t.Fatal("answered a channel ask with no linked channel")
	}
	if _, err := s.Wait(a.ID, "other-tab", time.Second); err != ErrNoAsk {
		t.Fatalf("another tab's wait = %v, want ErrNoAsk", err)
	}

	got := make(chan Ask)
	go func() {
		w, _ := s.Wait(a.ID, "t", time.Minute)
		got <- w
	}()
	waitFor(t, func() bool { _, blocking, _ := s.Count(); return blocking == 1 })
	if err := s.LinkChannel(a.ID, AskDB, "chan-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerChannel(a.ID, "ab12cd"); err != nil {
		t.Fatal(err)
	}
	w := <-got
	if w.State != AskAnswered || w.ShortID != "ab12cd" {
		t.Fatalf("waiter got %+v", w)
	}
	if err := s.Ack(a.ID, "other-tab"); err != ErrNoAsk {
		t.Fatalf("another tab's ack = %v", err)
	}
	if err := s.Ack(a.ID, "t"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * s.grace)
	if ids := rec.undeliveredIDs(); len(ids) != 0 {
		t.Fatalf("an acknowledged outcome was also reported undelivered: %v", ids)
	}
	if err := s.Decline(a.ID, "late"); err != ErrAskNotWaiting {
		t.Fatalf("declining an answered ask = %v", err)
	}
}

func TestAskLateAnswerUndelivered(t *testing.T) {
	s, _, rec := newTestAskStore()
	a, _ := s.Create(commandAsk("t", "git push"))
	w, err := s.Wait(a.ID, "t", 10*time.Millisecond)
	if err != nil || w.State != AskWaiting || w.Blocking {
		t.Fatalf("timed-out wait = %+v, %v", w, err)
	}
	if err := s.Decline(a.ID, "not now"); err != nil {
		t.Fatal(err)
	}
	if ids := rec.undeliveredIDs(); len(ids) != 1 || ids[0] != a.ID {
		t.Fatalf("undelivered = %v, want [%s]", ids, a.ID)
	}
	r, err := s.Result(a.ID, "t")
	if err != nil || r.State != AskDeclined || r.Note != "not now" {
		t.Fatalf("Result = %+v, %v", r, err)
	}
	if _, err := s.Result(a.ID, "other"); err != ErrNoAsk {
		t.Fatalf("another tab's Result = %v", err)
	}
}

func TestAskUnacknowledgedOutcomeIsNudged(t *testing.T) {
	s, _, rec := newTestAskStore()
	a, _ := s.Create(commandAsk("t", "ls"))
	got := make(chan Ask)
	go func() {
		w, _ := s.Wait(a.ID, "t", time.Minute)
		got <- w
	}()
	waitFor(t, func() bool { _, blocking, _ := s.Count(); return blocking == 1 })
	_ = s.Decline(a.ID, "no")
	<-got
	if ids := rec.undeliveredIDs(); len(ids) != 0 {
		t.Fatalf("reported before the grace period: %v", ids)
	}
	waitFor(t, func() bool { return len(rec.undeliveredIDs()) == 1 })
}

func TestAskWaiterWriteFailed(t *testing.T) {
	s, _, rec := newTestAskStore()
	a, _ := s.Create(commandAsk("t", "ls"))
	_ = s.Decline(a.ID, "")
	rec.undelivered = nil
	w, _ := s.Wait(a.ID, "t", time.Second)
	if w.State != AskDeclined {
		t.Fatalf("wait on a declined ask = %+v", w)
	}
	s.Delivered(a.ID, false)
	if ids := rec.undeliveredIDs(); len(ids) != 1 {
		t.Fatalf("a failed write was not reported undelivered: %v", ids)
	}
}

func TestAskCommandRun(t *testing.T) {
	s, _, rec := newTestAskStore()
	db, _ := s.Create(AskInput{Type: AskDB, TabID: "t", Cwd: "/w", Tables: []string{"a"}, Reason: "r"})
	if _, err := s.Start(db.ID); err == nil {
		t.Fatal("started a db ask")
	}
	a, _ := s.Create(commandAsk("t", "make"))
	if err := s.AnswerChannel(a.ID, "x"); err == nil {
		t.Fatal("answered a command ask with a channel")
	}
	started, err := s.Start(a.ID)
	if err != nil || started.State != AskRunning || started.Result == nil || started.Result.StartedAt.IsZero() {
		t.Fatalf("Start = %+v, %v", started, err)
	}
	if _, err := s.Start(a.ID); err != ErrAskNotWaiting {
		t.Fatalf("second Start = %v", err)
	}
	if waiting, _, _ := s.Count(); waiting != 1 {
		t.Fatalf("waiting = %d, want 1 (the db ask only)", waiting)
	}
	if list := s.List(); len(list) != 2 {
		t.Fatalf("List = %d asks, want 2", len(list))
	}
	s.Finish(a.ID, AskResult{ExitCode: 2, OutputTail: "boom"})
	got, _ := s.Get(a.ID)
	if got.State != AskAnswered || got.Result.ExitCode != 2 || got.Result.EndedAt.IsZero() || got.Result.StartedAt.IsZero() {
		t.Fatalf("finished = %+v %+v", got, got.Result)
	}
	if ids := rec.undeliveredIDs(); len(ids) != 1 {
		t.Fatalf("a finished run with no waiter was not reported: %v", ids)
	}

	b, _ := s.Create(commandAsk("t", "make b"))
	_, _ = s.Start(b.ID)
	s.Abort(b.ID)
	if got, _ := s.Get(b.ID); got.State != AskWaiting || got.Result != nil {
		t.Fatalf("aborted = %+v", got)
	}
}

func TestAskExpiry(t *testing.T) {
	s, clock, rec := newTestAskStore()
	a, _ := s.Create(commandAsk("t", "ls"))
	run, _ := s.Create(commandAsk("t", "make"))
	_, _ = s.Start(run.ID)
	got := make(chan Ask)
	go func() {
		w, _ := s.Wait(a.ID, "t", time.Minute)
		got <- w
	}()
	waitFor(t, func() bool { _, blocking, _ := s.Count(); return blocking == 1 })
	clock.advance(AskLifetime)
	s.Sweep()
	if w := <-got; w.State != AskExpired {
		t.Fatalf("waiter got %+v, want expired", w)
	}
	if _, err := s.Get(a.ID); err != ErrNoAsk {
		t.Fatalf("expired ask still held: %v", err)
	}
	if _, err := s.Get(run.ID); err != nil {
		t.Fatalf("a running command was dropped: %v", err)
	}
	if ids := rec.undeliveredIDs(); len(ids) != 0 {
		t.Fatalf("an expiry was reported as an answer: %v", ids)
	}
}

func TestAskCount(t *testing.T) {
	s, clock, _ := newTestAskStore()
	if w, b, id := s.Count(); w != 0 || b != 0 || id != "" {
		t.Fatalf("empty Count = %d %d %q", w, b, id)
	}
	_, _ = s.Create(commandAsk("t", "a"))
	clock.advance(time.Second)
	second, _ := s.Create(commandAsk("t", "b"))
	if w, _, id := s.Count(); w != 2 || id != second.ID {
		t.Fatalf("Count = %d %q, want 2 %q", w, id, second.ID)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
