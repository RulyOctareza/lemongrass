// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package agentusage records and reads the rate-limit usage that Claude Code and Codex hand over themselves: the
// statusline payload and the Codex rollout files. It never reads a credential or calls a vendor API.
package agentusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

// While the figures stay the same the file is still rewritten this often, so seen_at shows the feed is alive.
const refreshSeen = time.Minute

type Window struct {
	UsedPercent   float64 `json:"used_percent"`
	ResetsAt      int64   `json:"resets_at"`
	WindowMinutes int     `json:"window_minutes"`
}

type ClaudeRecord struct {
	FiveHour *Window `json:"five_hour,omitempty"`
	SevenDay *Window `json:"seven_day,omitempty"`
	SeenAt   int64   `json:"seen_at"`
}

func Dir(home string) string {
	return filepath.Join(home, ".lemongrass", "usage")
}

func claudePath(home, account string) string {
	return filepath.Join(Dir(home), "claude-"+account+".json")
}

type statuslineWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       int64    `json:"resets_at"`
}

func (w *statuslineWindow) window(minutes int) *Window {
	if w == nil || w.UsedPercentage == nil {
		return nil
	}
	return &Window{UsedPercent: *w.UsedPercentage, ResetsAt: w.ResetsAt, WindowMinutes: minutes}
}

// RecordClaude saves the rate_limits block of one statusline payload for the account. A payload without it, which
// Claude Code sends before the first API response and to non-subscribers, records nothing.
func RecordClaude(home, account string, payload []byte, now time.Time) error {
	var p struct {
		RateLimits *struct {
			FiveHour *statuslineWindow `json:"five_hour"`
			SevenDay *statuslineWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("agentusage: parsing statusline payload: %w", err)
	}
	if p.RateLimits == nil {
		return nil
	}
	rec := ClaudeRecord{
		FiveHour: p.RateLimits.FiveHour.window(300),
		SevenDay: p.RateLimits.SevenDay.window(10080),
		SeenAt:   now.Unix(),
	}
	if rec.FiveHour == nil && rec.SevenDay == nil {
		return nil
	}
	path := claudePath(home, account)
	if old, err := readClaude(path); err == nil {
		// Claude Code may report the two windows in separate payloads, so one left out keeps its last value until it
		// resets instead of being wiped.
		rec.FiveHour = keepUntilReset(rec.FiveHour, old.FiveHour, now)
		rec.SevenDay = keepUntilReset(rec.SevenDay, old.SevenDay, now)
		if sameWindows(old, rec) && now.Unix()-old.SeenAt < int64(refreshSeen/time.Second) {
			return nil
		}
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return agentconf.WriteAtomic(path, append(data, '\n'), 0o600)
}

func keepUntilReset(fresh, old *Window, now time.Time) *Window {
	if fresh != nil || old == nil || old.ResetsAt <= now.Unix() {
		return fresh
	}
	return old
}

func sameWindow(a, b *Window) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameWindows(a, b ClaudeRecord) bool {
	return sameWindow(a.FiveHour, b.FiveHour) && sameWindow(a.SevenDay, b.SevenDay)
}

func readClaude(path string) (ClaudeRecord, error) {
	var rec ClaudeRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("agentusage: parsing %s: %w", path, err)
	}
	return rec, nil
}

// ReadClaude returns the account's last record, and ok false when it has none yet.
func ReadClaude(home, account string) (ClaudeRecord, bool, error) {
	rec, err := readClaude(claudePath(home, account))
	if errors.Is(err, fs.ErrNotExist) {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	return rec, true, nil
}
