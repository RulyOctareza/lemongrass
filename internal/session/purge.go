// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import "fmt"

// purgeSteps run in order, children before the rows they hang off. Rows keyed only by tab id are deleted when no other
// project registered that tab, so a purge never reaches into another project's state.
var purgeSteps = []string{
	`DELETE FROM lg_message_mentions WHERE message_id IN (
		SELECT m.id FROM lg_messages m JOIN lg_threads t ON t.id = m.thread_id WHERE t.project_id = ?1)`,
	`DELETE FROM lg_thread_reads WHERE thread_id IN (SELECT id FROM lg_threads WHERE project_id = ?1)`,
	`DELETE FROM lg_messages WHERE thread_id IN (SELECT id FROM lg_threads WHERE project_id = ?1)`,
	`DELETE FROM lg_group_members WHERE group_id IN (SELECT id FROM lg_groups WHERE project_id = ?1)`,
	`DELETE FROM lg_tab_state WHERE tab_id IN (
		SELECT tab_id FROM lg_tabs WHERE project_id = ?1 UNION SELECT tab_id FROM lg_tab_sessions WHERE project_id = ?1)
		AND tab_id NOT IN (
		SELECT tab_id FROM lg_tabs WHERE project_id != ?1 UNION SELECT tab_id FROM lg_tab_sessions WHERE project_id != ?1)`,
	`DELETE FROM lg_notifications WHERE project_id = ?1`,
	`DELETE FROM lg_threads WHERE project_id = ?1`,
	`DELETE FROM lg_groups WHERE project_id = ?1`,
	`DELETE FROM lg_ready_marks WHERE project_id = ?1`,
	`DELETE FROM lg_tabs WHERE project_id = ?1`,
	`DELETE FROM lg_tab_sessions WHERE project_id = ?1`,
	`DELETE FROM lg_held_calls WHERE project_id = ?1`,
	`DELETE FROM lg_signatures WHERE project_id = ?1`,
	`DELETE FROM lg_tips WHERE project_id = ?1`,
	`DELETE FROM file_activity WHERE project_id = ?1`,
	`DELETE FROM sessions WHERE project_id = ?1`,
}

// PurgeProject deletes every row the store holds for the store's project, in one transaction. A table added with a
// project_id column, or keyed by a thread, group, message or tab of a project, needs its step here.
func (s *Store) PurgeProject() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, step := range purgeSteps {
		if _, err := tx.Exec(step, s.projectID); err != nil {
			return fmt.Errorf("session: purging project %s: %w", s.projectID, err)
		}
	}
	return tx.Commit()
}
