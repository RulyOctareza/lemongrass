// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package channelaudit is the shared lg_channel_audit table every vault-backed connector writes
// a call's outcome to. It lives in its own SQLite file under the vault's own directory, is
// written only by the vault-daemon side (dbgate.Gate, restergate.Gate), and prunes itself to a
// fixed 7-day window on every insert.
package channelaudit

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS lg_channel_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id TEXT NOT NULL,
	channel_kind TEXT NOT NULL,
	actor TEXT NOT NULL,
	action TEXT NOT NULL,
	target TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lg_channel_audit_created_at ON lg_channel_audit(created_at);
`

const retentionWindow = 7 * 24 * time.Hour

// Store holds the audit database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the audit database at dbPath, migrating its schema if needed.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("channelaudit: creating %s: %w", filepath.Dir(dbPath), err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("channelaudit: opening %s: %w", dbPath, err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("channelaudit: configuring %s: %w", dbPath, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("channelaudit: migrating schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Row is one call's outcome. Never a credential, a request body or a query string.
type Row struct {
	ChannelID   string
	ChannelKind string // "db" or "http"
	Actor       string
	Action      string
	Target      string
	Status      string
}

// Insert records row and purges every row older than the 7-day retention window as part of the
// same call, so a channel that goes quiet leaves its last week of rows until the next write
// anywhere touches the table.
func (s *Store) Insert(row Row) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("channelaudit: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO lg_channel_audit (channel_id, channel_kind, actor, action, target, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, row.ChannelID, row.ChannelKind, row.Actor, row.Action, row.Target, row.Status, now()); err != nil {
		return fmt.Errorf("channelaudit: inserting row: %w", err)
	}

	cutoff := time.Now().Add(-retentionWindow).UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`DELETE FROM lg_channel_audit WHERE created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("channelaudit: purging: %w", err)
	}

	return tx.Commit()
}

// RFC3339Nano, not RFC3339: second-only precision lets two writes in the same second compare equal.
func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
