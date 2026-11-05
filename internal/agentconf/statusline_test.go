// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func statusLineOf(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	line, _ := top["statusLine"].(map[string]any)
	return line
}

func TestReconcileStatusLineAdoptsHumanCommand(t *testing.T) {
	ours := StatusLineCommand(testLgrass, "primary")
	in := `{"model":"x","statusLine":{"type":"command","command":"~/bin/line.sh","padding":2}}`
	out, changed, keep, err := ReconcileStatusLine([]byte(in), ours, "")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if keep != "~/bin/line.sh" {
		t.Errorf("keep = %q, want the human's command", keep)
	}
	line := statusLineOf(t, out)
	if line["command"] != ours || line["padding"] != float64(2) || line["type"] != "command" {
		t.Errorf("statusLine = %v", line)
	}
	var top map[string]any
	json.Unmarshal(out, &top)
	if top["model"] != "x" {
		t.Error("other settings key lost")
	}
}

func TestReconcileStatusLineInstallsWhenAbsent(t *testing.T) {
	ours := StatusLineCommand(testLgrass, "work")
	out, changed, keep, err := ReconcileStatusLine([]byte(`{}`), ours, "")
	if err != nil || !changed || keep != "" {
		t.Fatalf("changed=%v keep=%q err=%v", changed, keep, err)
	}
	if statusLineOf(t, out)["command"] != ours {
		t.Error("ours not installed")
	}
}

func TestReconcileStatusLineIdempotentAndKeepsSaved(t *testing.T) {
	ours := StatusLineCommand(testLgrass, "primary")
	first, _, _, err := ReconcileStatusLine([]byte(`{"statusLine":{"type":"command","command":"line.sh"}}`), ours, "")
	if err != nil {
		t.Fatal(err)
	}
	second, changed, keep, err := ReconcileStatusLine(first, ours, "line.sh")
	if err != nil || changed || keep != "line.sh" || string(second) != string(first) {
		t.Fatalf("changed=%v keep=%q err=%v", changed, keep, err)
	}
}

func TestReconcileStatusLineRewritesOlderLgrassdPath(t *testing.T) {
	ours := StatusLineCommand(testLgrass, "primary")
	in := `{"statusLine":{"type":"command","command":"/old/lgrassd statusline --account primary"}}`
	out, changed, keep, err := ReconcileStatusLine([]byte(in), ours, "line.sh")
	if err != nil || !changed || keep != "line.sh" {
		t.Fatalf("changed=%v keep=%q err=%v", changed, keep, err)
	}
	if statusLineOf(t, out)["command"] != ours {
		t.Error("old path not rewritten")
	}
}

func TestReconcileStatusLineRestoresWhenBinaryAbsent(t *testing.T) {
	in := `{"statusLine":{"type":"command","command":"/x/lgrassd statusline --account primary","padding":1}}`
	out, changed, keep, err := ReconcileStatusLine([]byte(in), "", "line.sh")
	if err != nil || !changed || keep != "" {
		t.Fatalf("changed=%v keep=%q err=%v", changed, keep, err)
	}
	line := statusLineOf(t, out)
	if line["command"] != "line.sh" || line["padding"] != float64(1) {
		t.Errorf("statusLine = %v", line)
	}

	out, changed, _, err = ReconcileStatusLine([]byte(in), "", "")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if statusLineOf(t, out) != nil {
		t.Error("statusLine kept with no human command to restore")
	}
}

func TestReconcileStatusLineLeavesForeignWhenBinaryAbsent(t *testing.T) {
	in := `{"statusLine":{"type":"command","command":"line.sh"}}`
	out, changed, _, err := ReconcileStatusLine([]byte(in), "", "old.sh")
	if err != nil || changed || string(out) != in {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestReconcileStatusLineRefusesInvalidJSON(t *testing.T) {
	if _, _, _, err := ReconcileStatusLine([]byte(`{nope`), "x statusline --account primary", ""); err == nil {
		t.Error("invalid settings accepted")
	}
}

func TestSyncStatusLineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	store := StatusLineStore{Path: filepath.Join(dir, "statusline.json")}
	os.WriteFile(settings, []byte(`{"statusLine":{"type":"command","command":"line.sh"}}`), 0o644)

	if err := SyncStatusLine(settings, testLgrass, "primary", store); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Command("primary"); got != "line.sh" {
		t.Fatalf("stored %q", got)
	}
	data, _ := os.ReadFile(settings)
	if statusLineOf(t, data)["command"] != StatusLineCommand(testLgrass, "primary") {
		t.Fatal("ours not written")
	}

	if err := SyncStatusLine(settings, "", "primary", store); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settings)
	if statusLineOf(t, data)["command"] != "line.sh" {
		t.Error("human command not restored")
	}
	if got, _ := store.Command("primary"); got != "" {
		t.Errorf("entry kept after restore: %q", got)
	}
}

func TestStatusLineStorePrune(t *testing.T) {
	store := StatusLineStore{Path: filepath.Join(t.TempDir(), "statusline.json")}
	store.set("primary", "a.sh")
	store.set("gone", "b.sh")
	if err := store.Prune([]string{"primary"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Command("gone"); got != "" {
		t.Error("removed account kept")
	}
	if got, _ := store.Command("primary"); got != "a.sh" {
		t.Error("live account dropped")
	}
	info, _ := os.Stat(store.Path)
	// Windows keeps only the read-only bit of a mode.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
}
