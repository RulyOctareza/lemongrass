// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"database/sql"
	"time"
)

// A mark that a tab has loaded a skill.
func SkillMark(skill string) string { return "skill:" + skill }

// A mark that a collaborator has read its role with `lgrass how collaborator join`.
const CollaboratorMark = "how:collaborator-join"

// A mark that a tab has run a command a workgroup config required of it, normalized by NormalizeCommand.
func CommandMark(command string) string { return "cmd:" + command }

// A mark that a Claude thinker has read its role with ThinkerStartCommand.
var ThinkerStartMark = CommandMark(ThinkerStartCommand)

// A mark that a tab has read the close instructions for a scratchpad task.
func TaskCloseMark(slug string) string { return "task-close:" + slug }

// A mark that a tab was printed the close steps whose signature is given, so a repeat call can print only their titles.
func TaskCloseStepsMark(slug, signature string) string {
	return "task-close-steps:" + slug + ":" + signature
}

func (s *Store) MarkReady(tabID, kind string) error {
	_, err := s.db.Exec(`
		INSERT INTO lg_ready_marks (project_id, tab_id, kind, marked_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, tab_id, kind) DO UPDATE SET marked_at = excluded.marked_at
	`, s.projectID, tabID, kind, now())
	return err
}

func (s *Store) Marks(tabID string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT kind FROM lg_ready_marks WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, err
		}
		out[kind] = true
	}
	return out, rows.Err()
}

// MarkedAt is when the tab last recorded the mark; ok is false when it holds none.
func (s *Store) MarkedAt(tabID, kind string) (time.Time, bool, error) {
	var ts string
	err := s.db.QueryRow(`SELECT marked_at FROM lg_ready_marks WHERE project_id = ? AND tab_id = ? AND kind = ?`,
		s.projectID, tabID, kind).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	return t, err == nil, err
}

func (s *Store) ClearMarks(tabID string) error {
	_, err := s.db.Exec(`DELETE FROM lg_ready_marks WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID)
	return err
}
