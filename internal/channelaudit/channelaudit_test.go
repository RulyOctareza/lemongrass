// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package channelaudit

import (
	"path/filepath"
	"testing"
	"time"
)

func testRow() Row {
	return Row{
		ChannelID:   "chan-1",
		ChannelKind: "http",
		Actor:       "tab-1",
		Action:      "GET",
		Target:      "/api/orders",
		Status:      "200",
	}
}

func TestInsertAndRead(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Insert(testRow()); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lg_channel_audit`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var channelID, channelKind, actor, action, target, status string
	err = s.db.QueryRow(`SELECT channel_id, channel_kind, actor, action, target, status FROM lg_channel_audit`).
		Scan(&channelID, &channelKind, &actor, &action, &target, &status)
	if err != nil {
		t.Fatalf("reading row: %v", err)
	}
	want := testRow()
	if channelID != want.ChannelID || channelKind != want.ChannelKind || actor != want.Actor ||
		action != want.Action || target != want.Target || status != want.Status {
		t.Fatalf("row = %+v, want %+v", []string{channelID, channelKind, actor, action, target, status}, want)
	}
}

func TestInsertPurgesOldRows(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO lg_channel_audit (channel_id, channel_kind, actor, action, target, status, created_at)
		VALUES ('chan-old', 'db', 'tab-old', 'select', 't', '200', ?)
	`, old)
	if err != nil {
		t.Fatalf("seeding old row: %v", err)
	}

	if err := s.Insert(testRow()); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lg_channel_audit`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d after purge, want 1 (only the fresh insert)", count)
	}

	var channelID string
	if err := s.db.QueryRow(`SELECT channel_id FROM lg_channel_audit`).Scan(&channelID); err != nil {
		t.Fatalf("reading surviving row: %v", err)
	}
	if channelID != "chan-1" {
		t.Fatalf("surviving row channel_id = %q, want %q", channelID, "chan-1")
	}
}

func TestInsertKeepsRecentRows(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	recent := time.Now().Add(-6 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO lg_channel_audit (channel_id, channel_kind, actor, action, target, status, created_at)
		VALUES ('chan-recent', 'db', 'tab-recent', 'select', 't', '200', ?)
	`, recent)
	if err != nil {
		t.Fatalf("seeding recent row: %v", err)
	}

	if err := s.Insert(testRow()); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lg_channel_audit`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (recent row plus the fresh insert)", count)
	}
}

func TestOpenExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := s1.Insert(testRow()); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open on existing database: %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM lg_channel_audit`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (row survives reopen)", count)
	}
}
