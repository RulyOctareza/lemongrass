// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"database/sql"
	"time"
)

const (
	StateWorking   = "working"
	StateIdle      = "idle"
	StatePrompting = "prompting"

	// A working state nothing has refreshed for this long is treated as left over from a session that died.
	workingStaleAfter = 10 * time.Minute
)

// Which of the agent's own hook events last fired for the tab: a turn is running, the turn ended, or a permission prompt is open.
func (s *Store) SetTabState(tabID, state string) error {
	_, err := s.db.Exec(`
		INSERT INTO lg_tab_state (tab_id, state, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (tab_id) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
	`, tabID, state, now())
	return err
}

func (s *Store) ClearTabState(tabID string) error {
	_, err := s.db.Exec(`DELETE FROM lg_tab_state WHERE tab_id = ?`, tabID)
	return err
}

// True when the tab can take a typed nudge: its turn ended, its state is unknown, or a working state went stale. A permission prompt is never treated as stale.
func (s *Store) TabReadyForNudge(tabID string, at time.Time) (bool, error) {
	var state, updated string
	err := s.db.QueryRow(`SELECT state, updated_at FROM lg_tab_state WHERE tab_id = ?`, tabID).Scan(&state, &updated)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch state {
	case StateIdle:
		return true, nil
	case StateWorking:
		since, err := time.Parse(time.RFC3339Nano, updated)
		return err == nil && at.Sub(since) > workingStaleAfter, nil
	}
	return false, nil
}
