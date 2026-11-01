// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SourceAsk rows tell a tab that an ask it filed has its outcome. The ref is the ask id only: the outcome itself stays in the vault daemon and the model reads it with lgrass ask show, so no model or human text ever travels through the ledger.
const SourceAsk = "ask"

// InsertAskAnswered queues an ask row for tabID under the project the tab was last seen in, since the vault daemon that calls it is not tied to any project. It reports false when the tab is unknown.
func (s *Store) InsertAskAnswered(tabID, askID string) (bool, error) {
	if !validAskID(askID) {
		return false, fmt.Errorf("session: %q is not an ask id", askID)
	}
	var projectID string
	err := s.db.QueryRow(`SELECT project_id FROM lg_tabs WHERE tab_id = ? ORDER BY updated_at DESC LIMIT 1`, tabID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`
		INSERT INTO lg_notifications (project_id, source, ref, target_tab_id, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, projectID, SourceAsk, askID, tabID, KindYou, now())
	return err == nil, err
}

func validAskID(id string) bool {
	if id == "" || len(id) > 32 {
		return false
	}
	return strings.Trim(id, "abcdefghijklmnopqrstuvwxyz0123456789") == ""
}

// Composed from a fixed template; the ask id is checked to be lowercase letters and digits on insert.
func formatAskAnswered(ref string) string {
	if !validAskID(ref) {
		return Prefix + " an ask you filed has its answer. Read it with lgrass ask show <id>."
	}
	return fmt.Sprintf("%s ask %s has its answer. Read it with lgrass ask show %s.", Prefix, ref, ref)
}
