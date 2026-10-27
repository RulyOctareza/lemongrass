// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"context"
	"database/sql"
	"fmt"
)

// Satisfied by *sql.Conn and *sql.Tx, so a step runs on the connection that holds the migration's write lock.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// The step at index i takes the store from version i to i+1, kept in PRAGMA user_version. Steps are only ever appended: a released step is never edited, since stores already past it will not run it again.
var migrations = []func(execer) error{
	migrateV1,
	migrateV2,
}

// Version 1 is the schema as it stood before the store was versioned, applied idempotently so a store from any earlier release lands on the same tables.
func migrateV1(db execer) error {
	if err := dropLegacyThreads(db); err != nil {
		return err
	}
	if _, err := db.ExecContext(context.Background(), schema+threadSchema+legacyThreadDrop); err != nil {
		return err
	}
	return addMissingColumns(db)
}

// Version 2 makes the ledger hold notifications from any source: thread_id and message_id give way to source and ref, a thread row's ref being its message id.
func migrateV2(db execer) error {
	_, err := db.ExecContext(context.Background(), `
CREATE TABLE lg_notifications_v2 (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL,
	source TEXT NOT NULL,
	ref TEXT NOT NULL,
	target_tab_id TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'all',
	state TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	last_attempt_at TEXT,
	created_at TEXT NOT NULL,
	sent_at TEXT,
	read_at TEXT
);
INSERT INTO lg_notifications_v2 (id, project_id, source, ref, target_tab_id, kind, state, attempts, last_attempt_at, created_at, sent_at, read_at)
	SELECT id, project_id, 'thread', CAST(message_id AS TEXT), target_tab_id, kind, state, attempts, last_attempt_at, created_at, sent_at, read_at FROM lg_notifications;
DROP TABLE lg_notifications;
ALTER TABLE lg_notifications_v2 RENAME TO lg_notifications;
CREATE INDEX idx_lg_notifications_target ON lg_notifications(target_tab_id, state);
CREATE INDEX idx_lg_notifications_created ON lg_notifications(created_at);
CREATE INDEX idx_lg_notifications_source ON lg_notifications(source, ref);
`)
	return err
}

func userVersion(db execer) (int, error) {
	var v int
	err := db.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&v)
	return v, err
}

// Costs one version read on a current store. A store written by a newer lemongrass is refused rather than used with a schema this binary does not know.
func migrate(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	v, err := userVersion(conn)
	if err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("session: store is at version %d, this lemongrass knows up to version %d", v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		if err := applyStep(conn, i); err != nil {
			return fmt.Errorf("session: migrating to version %d: %w", i+1, err)
		}
	}
	return nil
}

// BEGIN IMMEDIATE takes the write lock up front, so of two processes opening the same old store one applies the step and the other, once it gets the lock, rereads the version and skips it.
func applyStep(conn *sql.Conn, i int) (err error) {
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()
	v, err := userVersion(conn)
	if err != nil {
		return err
	}
	if v > i {
		_, err = conn.ExecContext(ctx, `COMMIT`)
		return err
	}
	if err = migrations[i](conn); err != nil {
		return err
	}
	// PRAGMA takes no bound parameters; i+1 is an int, never outside input.
	if _, err = conn.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `COMMIT`)
	return err
}
