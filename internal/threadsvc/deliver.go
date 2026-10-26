// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package threadsvc delivers thread notifications: the ledger-driven delivery routines and the socket service the agent process hosts.
package threadsvc

import (
	"time"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/workgroup"
)

const (
	// Five attempts within roughly ten minutes with the backoff below, then the row stays pending for a hook.
	MaxAttempts = 5

	baseBackoff = 30 * time.Second
)

// The wait before attempt n+1 once n attempts have been made.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		return 0
	}
	return baseBackoff << (attempts - 1)
}

type Deliverer struct {
	Store *session.Store
	// Asks the app to type the tab's nudge into its terminal. The app composes the line and marks the rows sent.
	Nudge func(tabID string) (typed bool, reason string, err error)
	// Set by the service only. Ready reports a tab whose agent is between turns with no prompt open, and Later asks for another look at a tab that is not ready.
	Ready func(tabID string) bool
	Later func(tabID string)
}

func NewDeliverer(store *session.Store) *Deliverer {
	return &Deliverer{Store: store, Nudge: func(tabID string) (bool, string, error) {
		return workgroup.Nudge(workgroup.AppSocketPath(), tabID)
	}}
}

// Types a coalesced nudge into a supported agent's terminal when it is between turns.
func (d *Deliverer) DeliverTab(tabID string) (bool, error) {
	vendor, err := d.Store.VendorForTab(tabID)
	if err != nil || (vendor != "claude" && vendor != "codex") {
		return false, err
	}
	pending, err := d.Store.PendingForTab(tabID)
	if err != nil || len(pending) == 0 {
		return false, err
	}
	if d.Ready != nil && !d.Ready(tabID) {
		d.later(tabID)
		return false, nil
	}

	typed, _, err := d.Nudge(tabID)
	if err != nil {
		var rowIDs []int64
		for _, p := range pending {
			rowIDs = append(rowIDs, p.RowIDs...)
		}
		return false, d.Store.RecordNotificationAttempt(rowIDs)
	}
	if !typed {
		d.later(tabID)
	}
	return typed, nil
}

func (d *Deliverer) later(tabID string) {
	if d.Later != nil {
		d.Later(tabID)
	}
}

// The direct path a posting command takes when the service is unreachable.
func (d *Deliverer) DeliverPending() {
	tabs, err := d.Store.PendingTabs()
	if err != nil {
		return
	}
	for _, tab := range tabs {
		d.DeliverTab(tab)
	}
}
