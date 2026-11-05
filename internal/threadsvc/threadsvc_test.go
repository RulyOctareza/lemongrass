// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package threadsvc

import (
	"context"
	"errors"
	"github.com/faizalv/lemongrass/internal/ipc"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	tabClaude = "aaaaaaaa-1111-4111-8111-111111111111"
	tabCodex  = "bbbbbbbb-2222-4222-8222-222222222222"
	tabAuthor = "cccccccc-3333-4333-8333-333333333333"
)

func openStore(t *testing.T) *session.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := session.Open(session.DBPath(), "proj")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	store.RegisterTab(tabClaude, "claude")
	store.RegisterTab(tabCodex, "codex")
	store.RegisterTab(tabAuthor, "codex")
	return store
}

// Stands in for the app: types the tab's owed nudge, composed and marked typed the way `lgrassd nudge` does.
type recorder struct {
	mu    sync.Mutex
	store *session.Store
	texts []string
	fail  bool
	human bool
}

func (r *recorder) nudge(tab string) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return false, "", errors.New("the app is not reachable")
	}
	if r.human {
		return false, "typing", nil
	}
	text, ok := r.store.ConsumePending(tab)
	if !ok {
		return false, "nothing pending", nil
	}
	r.texts = append(r.texts, text)
	return true, "", nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.texts)
}

func (r *recorder) set(fail, human bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail, r.human = fail, human
}

func TestDeliverTabTypesOneCoalescedNudgeAndMarksSent(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			id, _ := store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			store.PostMessage(tabAuthor, id, "b !>>"+tab+"<<!")

			rec := &recorder{store: store}
			d := &Deliverer{Store: store, Nudge: rec.nudge}
			typed, err := d.DeliverTab(tab)
			if err != nil || !typed {
				t.Fatalf("DeliverTab = %v, %v, want a typed nudge", typed, err)
			}
			if rec.count() != 1 || !strings.Contains(rec.texts[0], ": 2 for you") || strings.Contains(rec.texts[0], "a !>>") {
				t.Errorf("typed %q, want one coalesced nudge without content", rec.texts)
			}
			if pending, _ := store.PendingForTab(tab); !pending.Empty() {
				t.Errorf("rows still pending after a nudge: %+v", pending)
			}
			if typed, _ := d.DeliverTab(tab); typed || rec.count() != 1 {
				t.Error("a second delivery typed again")
			}
		})
	}
}

// A branch row alone wakes an idle tab, and the typed line is the branch line.
func TestDeliverTabTypesABranchNotice(t *testing.T) {
	store := openStore(t)
	store.SetTabState(tabCodex, session.StateIdle)
	if _, err := store.InsertBranchChanged(session.BranchChange{From: "main", To: "fix-1", Carry: session.CarryCarried}, time.Now()); err != nil {
		t.Fatalf("InsertBranchChanged: %v", err)
	}
	rec := &recorder{store: store}
	d := &Deliverer{Store: store, Nudge: rec.nudge}
	if typed, err := d.DeliverTab(tabCodex); err != nil || !typed {
		t.Fatalf("DeliverTab = %v, %v, want a typed nudge", typed, err)
	}
	if rec.count() != 1 || !strings.HasPrefix(rec.texts[0], "[lg] branch changed from main to fix-1, uncommitted changes carried over.") {
		t.Errorf("typed %q, want the branch line", rec.texts)
	}
	if typed, _ := d.DeliverTab(tabClaude); typed {
		t.Error("a tab with no recent hook activity was nudged")
	}
}

// A tab with no process makes the app fail, and its rows must stay owed for the next tick however long it takes to start.
func TestDeliverTabFailureLeavesTheRowsOwed(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			rec := &recorder{store: store, fail: true}
			d := &Deliverer{Store: store, Nudge: rec.nudge}

			if typed, _ := d.DeliverTab(tab); typed {
				t.Error("reported a nudge that failed")
			}
			if pending, _ := store.PendingForTab(tab); len(pending.Threads) != 1 {
				t.Fatalf("pending = %+v, want the row kept", pending)
			}
			for i := 0; i < 10; i++ {
				d.DeliverTab(tab)
			}
			if tabs, _ := store.PendingTabs(); len(tabs) != 1 || tabs[0] != tab {
				t.Errorf("PendingTabs after repeated failures = %v, want the tab still owed", tabs)
			}
		})
	}
}

func TestDeliverTabLeavesUnsupportedTabsPending(t *testing.T) {
	store := openStore(t)
	store.RegisterTab(tabCodex, "cursor")
	store.CreateThread(tabAuthor, "Review", "a !>>"+tabCodex+"<<!")
	rec := &recorder{store: store}
	d := &Deliverer{Store: store, Nudge: rec.nudge}

	if typed, err := d.DeliverTab(tabCodex); typed || err != nil || rec.count() != 0 {
		t.Errorf("unsupported tab: typed %v, err %v, nudges %d, want none", typed, err, rec.count())
	}
	if pending, _ := store.PendingForTab(tabCodex); len(pending.Threads) != 1 {
		t.Error("unsupported row not left pending")
	}
}

func TestDeliverPendingNudgesBothSupportedVendors(t *testing.T) {
	store := openStore(t)
	if _, err := store.CreateThread(tabAuthor, "Review", "Check !>>"+tabClaude+"<<! !>>"+tabCodex+"<<!"); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{store: store}
	d := &Deliverer{Store: store, Nudge: rec.nudge}
	d.DeliverPending()
	if rec.count() != 2 {
		t.Fatalf("nudges = %d, want both supported tabs", rec.count())
	}
	for _, tab := range []string{tabClaude, tabCodex} {
		if pending, err := store.PendingForTab(tab); err != nil || !pending.Empty() {
			t.Errorf("tab %s pending = %+v, err = %v", tab, pending, err)
		}
	}
	d.DeliverPending()
	if rec.count() != 2 {
		t.Error("direct delivery repeated a sent nudge")
	}
}

func startService(t *testing.T, store *session.Store, rec *recorder) (*Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "threads.sock")
	l, err := ipc.Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	svc := NewService(store)
	if rec != nil {
		svc.deliver.Nudge = rec.nudge
	}
	go svc.Serve(l)
	return svc, path
}

func waitForCount(rec *recorder, want int) bool {
	deadline := time.Now().Add(3 * time.Second)
	for rec.count() < want && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	return rec.count() >= want
}

func TestWakeTypesTheNudgeForIdleSupportedTabs(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			rec := &recorder{store: store}
			_, path := startService(t, store, rec)

			if err := Wake(path); err != nil {
				t.Fatalf("Wake: %v", err)
			}
			if !waitForCount(rec, 1) || !strings.Contains(rec.texts[0], ": 1 for you") {
				t.Fatalf("typed = %q, want the nudge", rec.texts)
			}
		})
	}
}

func TestWakeReportsAnUnreachableService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none.sock")
	if err := Wake(path); err == nil {
		t.Error("Wake reported success with no service")
	}
}

func TestRunScansAtStartAndRetriesDueRows(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			rec := &recorder{store: store, fail: true}
			svc := NewService(store)
			svc.deliver.Nudge = rec.nudge
			svc.now = func() time.Time { return time.Now().Add(time.Hour) }

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { svc.Run(ctx, 60*time.Millisecond, time.Hour); close(done) }()

			time.Sleep(15 * time.Millisecond)
			rec.set(false, false)
			ok := waitForCount(rec, 1)
			cancel()
			<-done
			if !ok || rec.count() != 1 {
				t.Errorf("nudges after retry = %d, want 1", rec.count())
			}
			if pending, _ := store.PendingForTab(tab); !pending.Empty() {
				t.Error("row still pending after a successful retry")
			}
		})
	}
}

// The agent queues typed input while it works, so a turn in progress does not hold the line.
func TestATabMidTurnIsNudgedAtOnce(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			store.SetTabState(tab, session.StateWorking)

			rec := &recorder{store: store}
			svc := NewService(store)
			svc.deliver.Nudge = rec.nudge

			svc.Wake()
			if rec.count() != 1 {
				t.Fatalf("nudges for a tab mid-turn = %d, want 1", rec.count())
			}
		})
	}
}

func TestEveryDecisionIsLogged(t *testing.T) {
	store := openStore(t)
	store.CreateThread(tabAuthor, "Review", "a !>>"+tabClaude+"<<!")
	rec := &recorder{store: store}
	var outcomes []string
	d := &Deliverer{Store: store, Nudge: rec.nudge, Log: func(_ string, rows []int64, outcome string) {
		if len(rows) == 0 {
			t.Errorf("%q logged with no rows", outcome)
		}
		outcomes = append(outcomes, outcome)
	}}

	d.Ready = func(string) bool { return false }
	d.DeliverTab(tabClaude)
	d.Ready = nil
	rec.set(true, false)
	d.DeliverTab(tabClaude)
	rec.set(false, true)
	d.DeliverTab(tabClaude)
	rec.set(false, false)
	d.DeliverTab(tabClaude)

	want := []string{"held, a prompt is open", "not typed: the app is not reachable", "held, the human is typing", "typed"}
	if strings.Join(outcomes, "|") != strings.Join(want, "|") {
		t.Errorf("outcomes = %q, want %q", outcomes, want)
	}
}

func TestATabShowingAPermissionPromptIsNeverNudged(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			store.SetTabState(tab, session.StatePrompting)

			rec := &recorder{store: store}
			svc := NewService(store)
			svc.deliver.Nudge = rec.nudge
			svc.recheckDelay = 30 * time.Millisecond
			svc.now = func() time.Time { return time.Now().Add(24 * time.Hour) }

			svc.Wake()
			time.Sleep(300 * time.Millisecond)
			if rec.count() != 0 {
				t.Errorf("nudged a tab with an open permission prompt: %q", rec.texts)
			}
		})
	}
}

func TestANudgeWaitsWhileTheHumanIsTyping(t *testing.T) {
	for _, tab := range []string{tabClaude, tabCodex} {
		t.Run(tab, func(t *testing.T) {
			store := openStore(t)
			store.CreateThread(tabAuthor, "Review", "a !>>"+tab+"<<!")
			rec := &recorder{store: store, human: true}
			svc := NewService(store)
			svc.deliver.Nudge = rec.nudge
			svc.recheckDelay = 50 * time.Millisecond

			svc.Wake()
			time.Sleep(150 * time.Millisecond)
			if rec.count() != 0 {
				t.Fatalf("typed while the human was typing: %q", rec.texts)
			}
			rec.set(false, false)
			if !waitForCount(rec, 1) {
				t.Fatal("no nudge once the human stopped typing")
			}
		})
	}
}
