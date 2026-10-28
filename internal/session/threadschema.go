// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import "context"

const threadSchema = `
CREATE TABLE IF NOT EXISTS lg_tabs (
	project_id TEXT NOT NULL,
	tab_id TEXT NOT NULL,
	vendor TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (project_id, tab_id)
);
CREATE TABLE IF NOT EXISTS lg_threads (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	title TEXT NOT NULL,
	group_id INTEGER,
	created_by TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lg_threads_group ON lg_threads(group_id) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_lg_threads_project ON lg_threads(project_id, id);
CREATE TABLE IF NOT EXISTS lg_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	thread_id TEXT NOT NULL,
	tab_id TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lg_messages_thread ON lg_messages(thread_id, id);
CREATE TABLE IF NOT EXISTS lg_message_mentions (
	message_id INTEGER NOT NULL,
	tab_id TEXT NOT NULL,
	PRIMARY KEY (message_id, tab_id)
);
CREATE INDEX IF NOT EXISTS idx_lg_message_mentions_tab ON lg_message_mentions(tab_id);
CREATE TABLE IF NOT EXISTS lg_notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL,
	thread_id TEXT NOT NULL,
	message_id INTEGER NOT NULL,
	target_tab_id TEXT NOT NULL,
	state TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	last_attempt_at TEXT,
	created_at TEXT NOT NULL,
	sent_at TEXT,
	read_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_lg_notifications_target ON lg_notifications(target_tab_id, state);
CREATE INDEX IF NOT EXISTS idx_lg_notifications_created ON lg_notifications(created_at);
CREATE TABLE IF NOT EXISTS lg_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL,
	leader_tab_id TEXT NOT NULL,
	created_at TEXT NOT NULL,
	disbanded_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_lg_groups_project ON lg_groups(project_id, disbanded_at);
CREATE TABLE IF NOT EXISTS lg_group_members (
	group_id INTEGER NOT NULL,
	tab_id TEXT NOT NULL,
	role TEXT NOT NULL,
	label TEXT NOT NULL,
	vendor TEXT NOT NULL,
	PRIMARY KEY (group_id, tab_id)
);
CREATE INDEX IF NOT EXISTS idx_lg_group_members_tab ON lg_group_members(tab_id);
CREATE TABLE IF NOT EXISTS lg_ready_marks (
	project_id TEXT NOT NULL,
	tab_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	marked_at TEXT NOT NULL,
	PRIMARY KEY (project_id, tab_id, kind)
);
CREATE TABLE IF NOT EXISTS lg_thread_reads (
	tab_id TEXT NOT NULL,
	thread_id TEXT NOT NULL,
	last_message_id INTEGER NOT NULL,
	PRIMARY KEY (tab_id, thread_id)
);
CREATE TABLE IF NOT EXISTS lg_tab_state (
	tab_id TEXT PRIMARY KEY,
	state TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`

// Columns added after their table first shipped and before the store was versioned, part of version 1. A later column is a new migration step.
var addedColumns = []struct{ table, column, definition string }{
	{"lg_group_members", "prompt", "TEXT NOT NULL DEFAULT ''"},
	{"lg_group_members", "skills", "TEXT NOT NULL DEFAULT ''"},
	{"lg_notifications", "kind", "TEXT NOT NULL DEFAULT 'all'"},
	{"lg_tabs", "title", "TEXT NOT NULL DEFAULT ''"},
}

func addMissingColumns(db execer) error {
	ctx := context.Background()
	for _, c := range addedColumns {
		rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, c.table)
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return err
			}
			if name == c.column {
				found = true
			}
		}
		rows.Close()
		if found {
			continue
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE `+c.table+` ADD COLUMN `+c.column+` `+c.definition); err != nil {
			return err
		}
	}
	return nil
}

const legacyThreadDrop = `
DROP TABLE IF EXISTS thread_messages;
DROP TABLE IF EXISTS thread_participants;
DROP TABLE IF EXISTS lg_listener_heartbeats;
`

// Threads keyed by number and groups with pilot columns come from before the 6 character thread ids and the leader and thinker names; their tables are dropped rather than migrated.
func dropLegacyThreads(db execer) error {
	ctx := context.Background()
	var n int
	err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM pragma_table_info('lg_groups') WHERE name = 'pilot_tab_id')
		+ (SELECT COUNT(*) FROM pragma_table_info('lg_threads') WHERE name = 'id' AND type = 'INTEGER')`).Scan(&n)
	if err != nil || n == 0 {
		return err
	}
	_, err = db.ExecContext(ctx, `DROP TABLE IF EXISTS lg_groups; DROP TABLE IF EXISTS lg_group_members; DROP TABLE IF EXISTS lg_threads; DROP TABLE IF EXISTS lg_messages;
		DROP TABLE IF EXISTS lg_message_mentions; DROP TABLE IF EXISTS lg_notifications; DROP TABLE IF EXISTS lg_thread_reads;`)
	return err
}
