// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"encoding/json"
	"fmt"
	"time"
)

// What happened to the uncommitted changes when the git panel moved the branch.
const (
	CarryNone     = "none"     // the tree was clean
	CarryCarried  = "carried"  // the changes were stashed and applied on the new branch
	CarryConflict = "conflict" // the apply conflicted and the stash was kept
)

// Only tabs whose agent hooks fired this recently are told, since an idle tab will see the branch when it next looks.
const BranchNoticeWindow = 5 * time.Minute

// The ref of a branch row.
type BranchChange struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Carry string `json:"carry"`
}

func ValidCarry(carry string) bool {
	return carry == CarryNone || carry == CarryCarried || carry == CarryConflict
}

// Inserts one branch row for each Claude or Codex tab of the project that was active within the window, and returns those tabs.
func (s *Store) InsertBranchChanged(change BranchChange, at time.Time) ([]string, error) {
	if !ValidCarry(change.Carry) {
		return nil, fmt.Errorf("session: unknown carry %q, want none, carried or conflict", change.Carry)
	}
	ref, err := json.Marshal(change)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	cutoff := at.Add(-BranchNoticeWindow).UTC().Format(time.RFC3339Nano)
	rows, err := tx.Query(`
		SELECT t.tab_id FROM lg_tabs t JOIN lg_tab_state st ON st.tab_id = t.tab_id
		WHERE t.project_id = ? AND t.vendor IN ('claude', 'codex') AND st.updated_at > ?
		ORDER BY t.tab_id
	`, s.projectID, cutoff)
	if err != nil {
		return nil, err
	}
	var tabs []string
	for rows.Next() {
		var tab string
		if err := rows.Scan(&tab); err != nil {
			rows.Close()
			return nil, err
		}
		tabs = append(tabs, tab)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ts := now()
	for _, tab := range tabs {
		if _, err := tx.Exec(`
			INSERT INTO lg_notifications (project_id, source, ref, target_tab_id, kind, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, s.projectID, SourceBranch, string(ref), tab, KindYou, ts); err != nil {
			return nil, err
		}
	}
	return tabs, tx.Commit()
}

// Composed from a fixed template, never from model text. Branch names pass git's ref format check, so they hold no spaces or control characters.
func formatBranchChange(ref string) string {
	var c BranchChange
	if err := json.Unmarshal([]byte(ref), &c); err != nil {
		return Prefix + " the git branch changed. Check git status before you continue."
	}
	middle := ""
	switch c.Carry {
	case CarryCarried:
		middle = ", uncommitted changes carried over"
	case CarryConflict:
		middle = ", the carried changes conflicted and the stash was kept"
	}
	return fmt.Sprintf("%s branch changed from %s to %s%s. Check git status before you continue.", Prefix, c.From, c.To, middle)
}

// One line per event row, after the thread lines.
func formatEvent(e PendingEvent) string {
	switch e.Source {
	case SourceBranch:
		return formatBranchChange(e.Ref)
	}
	return fmt.Sprintf("%s a %s notification arrived.", Prefix, e.Source)
}
