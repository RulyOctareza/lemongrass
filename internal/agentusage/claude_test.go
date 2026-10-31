// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"os"
	"testing"
	"time"
)

const payload = `{"session_id":"s","rate_limits":{"five_hour":{"used_percentage":34.5,"resets_at":1793173300},"seven_day":{"used_percentage":12,"resets_at":1793688948}}}`

func TestRecordClaudeSavesWindows(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(1793170000, 0)
	if err := RecordClaude(home, "work", []byte(payload), now); err != nil {
		t.Fatal(err)
	}
	rec, ok, err := ReadClaude(home, "work")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if rec.FiveHour == nil || rec.FiveHour.UsedPercent != 34.5 || rec.FiveHour.WindowMinutes != 300 || rec.FiveHour.ResetsAt != 1793173300 {
		t.Errorf("five_hour = %+v", rec.FiveHour)
	}
	if rec.SevenDay == nil || rec.SevenDay.UsedPercent != 12 || rec.SevenDay.WindowMinutes != 10080 {
		t.Errorf("seven_day = %+v", rec.SevenDay)
	}
	if rec.SeenAt != now.Unix() {
		t.Errorf("seen_at = %d", rec.SeenAt)
	}
	if _, ok, _ := ReadClaude(home, "primary"); ok {
		t.Error("another account got a record")
	}
}

func TestRecordClaudeIgnoresPayloadWithoutRateLimits(t *testing.T) {
	home := t.TempDir()
	if err := RecordClaude(home, "primary", []byte(`{"session_id":"s"}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Dir(home)); err == nil {
		t.Error("usage written for a payload without rate_limits")
	}
}

func TestRecordClaudeSkipsUnchangedWithinAMinute(t *testing.T) {
	home := t.TempDir()
	start := time.Unix(1793170000, 0)
	RecordClaude(home, "primary", []byte(payload), start)
	RecordClaude(home, "primary", []byte(payload), start.Add(30*time.Second))
	rec, _, _ := ReadClaude(home, "primary")
	if rec.SeenAt != start.Unix() {
		t.Error("unchanged figures rewritten within a minute")
	}
	RecordClaude(home, "primary", []byte(payload), start.Add(61*time.Second))
	rec, _, _ = ReadClaude(home, "primary")
	if rec.SeenAt != start.Add(61*time.Second).Unix() {
		t.Error("seen_at not refreshed after a minute")
	}
}

func TestRecordClaudeRejectsBadJSON(t *testing.T) {
	if err := RecordClaude(t.TempDir(), "primary", []byte(`{`), time.Now()); err == nil {
		t.Error("bad payload accepted")
	}
}

func TestRecordClaudeKeepsWindowLeftOutUntilItResets(t *testing.T) {
	home := t.TempDir()
	start := time.Unix(1793170000, 0)
	RecordClaude(home, "primary", []byte(payload), start)
	fiveOnly := `{"rate_limits":{"five_hour":{"used_percentage":40,"resets_at":1793173300}}}`
	if err := RecordClaude(home, "primary", []byte(fiveOnly), start.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	rec, _, _ := ReadClaude(home, "primary")
	if rec.FiveHour.UsedPercent != 40 || rec.SevenDay == nil || rec.SevenDay.UsedPercent != 12 {
		t.Fatalf("five %+v seven %+v", rec.FiveHour, rec.SevenDay)
	}

	afterWeek := time.Unix(1793688948+1, 0)
	RecordClaude(home, "primary", []byte(`{"rate_limits":{"five_hour":{"used_percentage":1,"resets_at":1793700000}}}`), afterWeek)
	rec, _, _ = ReadClaude(home, "primary")
	if rec.SevenDay != nil {
		t.Errorf("seven_day kept past its reset: %+v", rec.SevenDay)
	}
}
