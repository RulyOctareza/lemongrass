// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"fmt"
	"github.com/faizalv/lemongrass/internal/ipc"
	"os"
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/threadsvc"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/workgroup"
)

const (
	askHintTimeout = 2 * time.Second
	askSweepEvery  = time.Minute
)

// askHinter tells the app that the asks changed. The hint is content-free, so the app refetches the count and list itself and a forged hint shows nothing new. Hints coalesce: while one is being sent, any number of changes produce one more.
type askHinter struct {
	mu      sync.Mutex
	sending bool
	again   bool
	send    func() error
}

func newAskHinter() *askHinter {
	return &askHinter{send: func() error { return sendAskHint(workgroup.AppSocketPath()) }}
}

func (h *askHinter) Notify() {
	h.mu.Lock()
	if h.sending {
		h.again = true
		h.mu.Unlock()
		return
	}
	h.sending = true
	h.mu.Unlock()
	go h.loop()
}

func (h *askHinter) loop() {
	for {
		_ = h.send()
		h.mu.Lock()
		if !h.again {
			h.sending = false
			h.mu.Unlock()
			return
		}
		h.again = false
		h.mu.Unlock()
	}
}

// sendAskHint writes {"op":"ask_hint"} to the app socket. An app that is not running misses nothing: it reads the count when it starts.
func sendAskHint(socketPath string) error {
	conn, err := ipc.Dial(socketPath, askHintTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(askHintTimeout))
	if err := json.NewEncoder(conn).Encode(map[string]string{"op": "ask_hint"}); err != nil {
		return err
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	return json.NewDecoder(conn).Decode(&resp)
}

// deliverAskAnswer queues a ledger row telling the tab its ask has its outcome and wakes delivery, the way lgrassd branch changed does. The row carries only the ask id.
func deliverAskAnswer(a vault.Ask) {
	store, err := session.Open(session.DBPath(), "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd vault: queueing the answer to ask %s: %v\n", a.ID, err)
		return
	}
	defer store.Close()
	queued, err := store.InsertAskAnswered(a.TabID, a.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd vault: queueing the answer to ask %s: %v\n", a.ID, err)
		return
	}
	if !queued {
		return
	}
	if threadsvc.Wake(threadsvc.SocketPath()) == nil {
		return
	}
	threadsvc.NewDeliverer(store).DeliverPending()
}

// wireAsks connects the ask store to the app and the ledger and starts the expiry sweep.
func wireAsks(asks *vault.AskStore) {
	hinter := newAskHinter()
	asks.OnChange = hinter.Notify
	asks.OnUndelivered = func(a vault.Ask) { go deliverAskAnswer(a) }
	go func() {
		ticker := time.NewTicker(askSweepEvery)
		defer ticker.Stop()
		for range ticker.C {
			asks.Sweep()
		}
	}()
}
