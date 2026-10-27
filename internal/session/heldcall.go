// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"database/sql"
	"errors"
	"time"
)

// HeldCallTTL bounds how long a denied call can be replayed after it was held.
const HeldCallTTL = 10 * time.Minute

// ErrNoHeldCall means no unexpired held call with that id exists for that session.
var ErrNoHeldCall = errors.New("session: no held call")

// HeldCall is a tool call a gate denied, stored as the raw hook payload so `lgrass replay` can run it without the model resending it.
type HeldCall struct {
	ID         string
	SessionID  string
	TabID      string
	ToolName   string
	Payload    []byte
	HookBinary string
	HeldAt     time.Time
}

// HoldCall stores c under a new id, which it returns, and prunes this project's expired held calls.
func (s *Store) HoldCall(c HeldCall) (string, error) {
	cutoff := time.Now().Add(-HeldCallTTL).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`DELETE FROM lg_held_calls WHERE project_id = ? AND held_at < ?`, s.projectID, cutoff); err != nil {
		return "", err
	}
	id, err := newShortID(s.db, `SELECT 1 FROM lg_held_calls WHERE id = ?`, "held call")
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`
		INSERT INTO lg_held_calls (project_id, id, session_id, tab_id, tool_name, payload, hook_binary, held_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.projectID, id, c.SessionID, c.TabID, c.ToolName, c.Payload, c.HookBinary, now())
	if err != nil {
		return "", err
	}
	return id, nil
}

// HeldCalls lists this session's unexpired held calls, oldest first.
func (s *Store) HeldCalls(sessionID string) ([]HeldCall, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, tab_id, tool_name, payload, hook_binary, held_at FROM lg_held_calls
		WHERE project_id = ? AND session_id = ? ORDER BY held_at
	`, s.projectID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeldCall
	for rows.Next() {
		c, err := scanHeldCall(rows)
		if err != nil {
			return nil, err
		}
		if time.Since(c.HeldAt) <= HeldCallTTL {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// HeldCall returns ErrNoHeldCall when the id is unknown, expired or held by another session.
func (s *Store) HeldCall(sessionID, id string) (HeldCall, error) {
	row := s.db.QueryRow(`
		SELECT id, session_id, tab_id, tool_name, payload, hook_binary, held_at FROM lg_held_calls
		WHERE project_id = ? AND session_id = ? AND id = ?
	`, s.projectID, sessionID, id)
	c, err := scanHeldCall(row)
	if err == sql.ErrNoRows {
		return HeldCall{}, ErrNoHeldCall
	}
	if err != nil {
		return HeldCall{}, err
	}
	if time.Since(c.HeldAt) > HeldCallTTL {
		return HeldCall{}, ErrNoHeldCall
	}
	return c, nil
}

// DropHeldCall reports false when another caller already dropped it, so only one replay of a held call runs.
func (s *Store) DropHeldCall(sessionID, id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM lg_held_calls WHERE project_id = ? AND session_id = ? AND id = ?`, s.projectID, sessionID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanHeldCall(r rowScanner) (HeldCall, error) {
	var c HeldCall
	var heldAt string
	if err := r.Scan(&c.ID, &c.SessionID, &c.TabID, &c.ToolName, &c.Payload, &c.HookBinary, &heldAt); err != nil {
		return HeldCall{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, heldAt)
	if err != nil {
		return HeldCall{}, err
	}
	c.HeldAt = t
	return c, nil
}
