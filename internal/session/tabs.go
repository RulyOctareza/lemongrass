// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"database/sql"
	"strings"
	"time"
	"unicode"
)

// The longest tab title kept, in runes.
const MaxTabTitleRunes = 80

// Control characters become spaces, runs of whitespace collapse, and the result is cut to MaxTabTitleRunes.
func CleanTabTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if r := []rune(title); len(r) > MaxTabTitleRunes {
		title = string(r[:MaxTabTitleRunes])
	}
	return title
}

// A tab record is refreshed on spawn and on hook activity, so one untouched for this long belongs to a tab that no longer exists.
const tabRecordTTL = 30 * 24 * time.Hour

func (s *Store) RecordTabSession(tabID, sessionID string) error {
	_, err := s.db.Exec(`
		INSERT INTO lg_tab_sessions (project_id, tab_id, session_id, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, tab_id) DO UPDATE SET
			session_id = excluded.session_id,
			updated_at = excluded.updated_at
	`, s.projectID, tabID, sessionID, now())
	return err
}

// Maps tab id to the tab's most recently reported session id.
func (s *Store) TabSessions() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT tab_id, session_id FROM lg_tab_sessions WHERE project_id = ?`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var tabID, sessionID string
		if err := rows.Scan(&tabID, &sessionID); err != nil {
			return nil, err
		}
		out[tabID] = sessionID
	}
	return out, rows.Err()
}

// Empty string, no error, when the tab has reported no session.
func (s *Store) SessionForTab(tabID string) (string, error) {
	var sessionID string
	err := s.db.QueryRow(`SELECT session_id FROM lg_tab_sessions WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sessionID, err
}

func (s *Store) ForgetTabSession(tabID string) error {
	_, err := s.db.Exec(`DELETE FROM lg_tab_sessions WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID)
	return err
}

// Also prunes every project's stale records, so a project that is never opened again does not keep its rows.
func (s *Store) RegisterTab(tabID, vendor string) error {
	ts := now()
	if _, err := s.db.Exec(`
		INSERT INTO lg_tabs (project_id, tab_id, vendor, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, tab_id) DO UPDATE SET
			vendor = excluded.vendor,
			updated_at = excluded.updated_at
	`, s.projectID, tabID, vendor, ts); err != nil {
		return err
	}
	cutoff := time.Now().Add(-tabRecordTTL).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`DELETE FROM lg_tabs WHERE updated_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM lg_ready_marks WHERE marked_at < ? AND tab_id NOT IN (SELECT tab_id FROM lg_tabs)`, cutoff)
	return err
}

// Updates only, so a tab that was never registered stays unrecorded. The title is the shell's own, which an agent may have written, so it is cleaned and capped on the way in.
func (s *Store) SetTabTitle(tabID, title string) error {
	_, err := s.db.Exec(`UPDATE lg_tabs SET title = ? WHERE project_id = ? AND tab_id = ?`, CleanTabTitle(title), s.projectID, tabID)
	return err
}

// Maps tab id to the tab's stored title for every registered tab in this project.
func (s *Store) TabTitles() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT tab_id, title FROM lg_tabs WHERE project_id = ?`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var tabID, title string
		if err := rows.Scan(&tabID, &title); err != nil {
			return nil, err
		}
		out[tabID] = title
	}
	return out, rows.Err()
}

// Updates only, so a tab that was never registered stays unrecorded.
func (s *Store) TouchTab(tabID string) error {
	_, err := s.db.Exec(`UPDATE lg_tabs SET updated_at = ? WHERE project_id = ? AND tab_id = ?`, now(), s.projectID, tabID)
	return err
}

// Empty string, no error, when the tab has no record.
func (s *Store) TabVendor(tabID string) (string, error) {
	var vendor string
	err := s.db.QueryRow(`SELECT vendor FROM lg_tabs WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID).Scan(&vendor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return vendor, err
}

func (s *Store) ForgetTab(tabID string) error {
	if err := s.ForgetTabSession(tabID); err != nil {
		return err
	}
	if err := s.ClearMarks(tabID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM lg_tabs WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID)
	return err
}

type TabInfo struct {
	ID     string
	Vendor string
	Title  string
}

// Every registered tab in this project, ordered by tab id so two calls never differ.
func (s *Store) ListTabs() ([]TabInfo, error) {
	rows, err := s.db.Query(`SELECT tab_id, vendor, title FROM lg_tabs WHERE project_id = ? ORDER BY tab_id`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TabInfo
	for rows.Next() {
		var t TabInfo
		if err := rows.Scan(&t.ID, &t.Vendor, &t.Title); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Drops every registered tab of this project and the state tied to them, but keeps each tab's saved session, which is what a restored tab resumes from.
func (s *Store) ClearTabs() error {
	if _, err := s.db.Exec(`DELETE FROM lg_tab_state WHERE tab_id IN (SELECT tab_id FROM lg_tabs WHERE project_id = ?)`, s.projectID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM lg_ready_marks WHERE project_id = ? AND tab_id IN (SELECT tab_id FROM lg_tabs WHERE project_id = ?)`, s.projectID, s.projectID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM lg_tabs WHERE project_id = ?`, s.projectID)
	return err
}

// Maps tab id to vendor for every registered tab in this project.
func (s *Store) TabVendors() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT tab_id, vendor FROM lg_tabs WHERE project_id = ?`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var tabID, vendor string
		if err := rows.Scan(&tabID, &vendor); err != nil {
			return nil, err
		}
		out[tabID] = vendor
	}
	return out, rows.Err()
}
