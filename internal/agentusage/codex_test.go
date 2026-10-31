// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentusage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const codexSnapshot = `{"timestamp":"2026-10-28T03:34:40.744Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex","primary":{"used_percent":9.0,"window_minutes":300,"resets_at":1793173300},"secondary":{"used_percent":3.0,"window_minutes":10080,"resets_at":1793688948},"plan_type":"plus"}}}`

func writeRollout(t *testing.T, codexHome, day, name, body string, mod time.Time) {
	t.Helper()
	dir := filepath.Join(codexHome, "sessions", day)
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, mod, mod)
}

func TestReadCodexTakesNewestSnapshot(t *testing.T) {
	home := t.TempDir()
	old := `{"timestamp":"2026-10-20T00:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":50,"window_minutes":300,"resets_at":1}}}}`
	writeRollout(t, home, "2026/10/20", "rollout-a.jsonl", old+"\n", time.Unix(1000, 0))
	writeRollout(t, home, "2026/10/28", "rollout-b.jsonl", `{"type":"session_meta"}`+"\n"+old+"\n"+codexSnapshot+"\n"+`{"type":"response_item"}`+"\n", time.Unix(2000, 0))

	rec, ok, err := ReadCodex(home)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(rec.Windows) != 2 || rec.Windows[0].UsedPercent != 9 || rec.Windows[1].WindowMinutes != 10080 {
		t.Errorf("windows = %+v", rec.Windows)
	}
	if rec.PlanType != "plus" || rec.SeenAt != time.Date(2026, 10, 28, 3, 34, 40, 0, time.UTC).Unix() {
		t.Errorf("plan %q seen %d", rec.PlanType, rec.SeenAt)
	}
}

func TestReadCodexFindsSessionStartedOnAnEarlierDay(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "2026/10/28", "rollout-new.jsonl", `{"type":"session_meta"}`+"\n", time.Unix(1000, 0))
	writeRollout(t, home, "2026/10/25", "rollout-long.jsonl", codexSnapshot+"\n", time.Unix(3000, 0))
	rec, ok, err := ReadCodex(home)
	if err != nil || !ok || rec.Windows[0].UsedPercent != 9 {
		t.Fatalf("ok=%v err=%v rec=%+v", ok, err, rec)
	}
}

func TestReadCodexWithoutSessions(t *testing.T) {
	if _, ok, err := ReadCodex(t.TempDir()); ok || err != nil {
		t.Errorf("ok=%v err=%v", ok, err)
	}
}

func TestCollectAppliesResetRule(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	writeRollout(t, codexHome, "2026/10/28", "rollout.jsonl", codexSnapshot+"\n", time.Unix(2000, 0))
	RecordClaude(home, "primary", []byte(payload), time.Unix(1793170000, 0))

	now := time.Unix(1793180000, 0) // past the 5h reset, before the 7d one
	report := Collect(home, codexHome, []string{"primary", "work"}, now)
	if len(report.Claude) != 1 || report.Claude[0].Account != "primary" {
		t.Fatalf("claude = %+v", report.Claude)
	}
	five, seven := report.Claude[0].Windows[0], report.Claude[0].Windows[1]
	if five.Label != "5h" || !five.Reset || five.UsedPercent != 0 {
		t.Errorf("5h = %+v", five)
	}
	if seven.Label != "7d" || seven.Reset || seven.UsedPercent != 12 {
		t.Errorf("7d = %+v", seven)
	}
	if report.Codex == nil || !report.Codex.Windows[0].Reset || report.Codex.Windows[1].UsedPercent != 3 {
		t.Errorf("codex = %+v", report.Codex)
	}
}

func TestLabel(t *testing.T) {
	for minutes, want := range map[int]string{300: "5h", 10080: "7d", 90: "90m", 43200: "30d", 0: "?"} {
		if got := Label(minutes); got != want {
			t.Errorf("Label(%d) = %q, want %q", minutes, got, want)
		}
	}
}
