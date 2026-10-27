// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store tracks one project's sessions and file activity, fed by Claude Code's hook events, not any live process state.
type Store struct {
	db        *sql.DB
	projectID string
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	project_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	last_activity_at TEXT NOT NULL,
	nudge_counter INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (project_id, session_id)
);
CREATE INDEX IF NOT EXISTS idx_sessions_open ON sessions(project_id, ended_at);
CREATE TABLE IF NOT EXISTS file_activity (
	project_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	file_path TEXT NOT NULL,
	dir TEXT NOT NULL,
	touched_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_activity_file ON file_activity(project_id, file_path, touched_at);
CREATE INDEX IF NOT EXISTS idx_file_activity_dir ON file_activity(project_id, dir, touched_at);
CREATE TABLE IF NOT EXISTS lg_tips (
	project_id TEXT NOT NULL,
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	message TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_lg_tips_project ON lg_tips(project_id);
CREATE TABLE IF NOT EXISTS lg_tab_sessions (
	project_id TEXT NOT NULL,
	tab_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (project_id, tab_id)
);
CREATE TABLE IF NOT EXISTS lg_signatures (
	project_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	checklist_id TEXT NOT NULL,
	signed_at TEXT NOT NULL,
	PRIMARY KEY (project_id, session_id, checklist_id)
);
CREATE TABLE IF NOT EXISTS lg_held_calls (
	project_id TEXT NOT NULL,
	id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tab_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	payload BLOB NOT NULL,
	hook_binary TEXT NOT NULL,
	held_at TEXT NOT NULL,
	PRIMARY KEY (project_id, id)
);
`

// Open may run before config.Dir() exists, since a hook can fire before any other lgrass call in a project. The app migrates the store at boot through `lgrassd migrate`; migrating here is the fallback for a CLI used before the app ever ran.
func Open(dbPath, projectID string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Dir(dbPath), err)
	}
	// Every hook in every live session opens this file concurrently; busy_timeout retries instead of erroring, WAL keeps readers from blocking on a writer.
	// Set in the DSN so every pooled connection gets them, the migrator's own included.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, projectID: projectID}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// RFC3339Nano, not RFC3339: second-only precision lets two writes in the same second compare equal.
func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Clears this session_id's checklist signatures, so a fresh or reused session starts ungated.
func (s *Store) Start(sessionID string) error {
	ts := now()
	if _, err := s.db.Exec(`DELETE FROM lg_signatures WHERE project_id = ? AND session_id = ?`, s.projectID, sessionID); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO sessions (project_id, session_id, started_at, ended_at, last_activity_at, nudge_counter)
		VALUES (?, ?, ?, NULL, ?, 0)
		ON CONFLICT (project_id, session_id) DO UPDATE SET
			started_at = excluded.started_at,
			ended_at = NULL,
			last_activity_at = excluded.last_activity_at,
			nudge_counter = 0
	`, s.projectID, sessionID, ts, ts)
	return err
}

func (s *Store) EnsureOpen(sessionID string) error {
	ts := now()
	_, err := s.db.Exec(`
		INSERT INTO sessions (project_id, session_id, started_at, ended_at, last_activity_at, nudge_counter)
		VALUES (?, ?, ?, NULL, ?, 0)
		ON CONFLICT (project_id, session_id) DO UPDATE SET
			ended_at = NULL,
			last_activity_at = excluded.last_activity_at
	`, s.projectID, sessionID, ts, ts)
	return err
}

// Upserts so a missed or misconfigured SessionStart hook doesn't leave liveness permanently blind to this session.
func (s *Store) Touch(sessionID string) error {
	ts := now()
	_, err := s.db.Exec(`
		INSERT INTO sessions (project_id, session_id, started_at, ended_at, last_activity_at, nudge_counter)
		VALUES (?, ?, ?, NULL, ?, 0)
		ON CONFLICT (project_id, session_id) DO UPDATE SET
			last_activity_at = excluded.last_activity_at
	`, s.projectID, sessionID, ts, ts)
	return err
}

func (s *Store) End(sessionID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET ended_at = ? WHERE project_id = ? AND session_id = ?`, now(), s.projectID, sessionID)
	return err
}

func (s *Store) LogFileActivity(sessionID, filePath string) error {
	_, err := s.db.Exec(`
		INSERT INTO file_activity (project_id, session_id, file_path, dir, touched_at)
		VALUES (?, ?, ?, ?, ?)
	`, s.projectID, sessionID, filePath, filepath.Dir(filePath), now())
	return err
}

type FileTouch struct {
	SessionID string
	FilePath  string
}

// Oldest first, so callers that keep the first touch per session get them in the order they started editing.
func (s *Store) FileActivitySince(since time.Time) ([]FileTouch, error) {
	rows, err := s.db.Query(`
		SELECT session_id, file_path FROM file_activity
		WHERE project_id = ? AND touched_at > ?
		ORDER BY touched_at
	`, s.projectID, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var touches []FileTouch
	for rows.Next() {
		var t FileTouch
		if err := rows.Scan(&t.SessionID, &t.FilePath); err != nil {
			return nil, err
		}
		touches = append(touches, t)
	}
	return touches, rows.Err()
}

type ActivityHit struct {
	SessionID string
	FilePath  string
	TouchedAt string
	SameFile  bool // false means a sibling file in the same directory, not the same file
}

// Deduplicated to one hit per session, most recent first.
func (s *Store) RecentActivity(excludeSessionID, filePath string, window time.Duration) ([]ActivityHit, error) {
	cutoff := time.Now().Add(-window).UTC().Format(time.RFC3339)
	dir := filepath.Dir(filePath)

	rows, err := s.db.Query(`
		SELECT fa.session_id, fa.file_path, fa.touched_at
		FROM file_activity fa
		JOIN sessions sess ON sess.project_id = fa.project_id AND sess.session_id = fa.session_id
		WHERE fa.project_id = ?
			AND fa.session_id != ?
			AND sess.ended_at IS NULL
			AND fa.touched_at > ?
			AND (fa.file_path = ? OR fa.dir = ?)
		ORDER BY fa.touched_at DESC
	`, s.projectID, excludeSessionID, cutoff, filePath, dir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var hits []ActivityHit
	for rows.Next() {
		var h ActivityHit
		if err := rows.Scan(&h.SessionID, &h.FilePath, &h.TouchedAt); err != nil {
			return nil, err
		}
		if seen[h.SessionID] {
			continue
		}
		seen[h.SessionID] = true
		h.SameFile = h.FilePath == filePath
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

type SessionStatus struct {
	SessionID string
	Active    bool // false means idling: no tool activity within the threshold
}

// len(result) is also the population count: how many other sessions are open right now.
func (s *Store) Liveness(excludeSessionID string, idleThreshold time.Duration) ([]SessionStatus, error) {
	cutoff := time.Now().Add(-idleThreshold).UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT session_id, last_activity_at FROM sessions
		WHERE project_id = ? AND ended_at IS NULL AND session_id != ?
		ORDER BY last_activity_at DESC
	`, s.projectID, excludeSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionStatus
	for rows.Next() {
		var id, lastActivity string
		if err := rows.Scan(&id, &lastActivity); err != nil {
			return nil, err
		}
		out = append(out, SessionStatus{SessionID: id, Active: lastActivity > cutoff})
	}
	return out, rows.Err()
}

// Upserts for the same reason Touch does.
func (s *Store) IncrementNudgeCounter(sessionID string, threshold int) (fire bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	ts := now()
	if _, err := tx.Exec(`
		INSERT INTO sessions (project_id, session_id, started_at, ended_at, last_activity_at, nudge_counter)
		VALUES (?, ?, ?, NULL, ?, 0)
		ON CONFLICT (project_id, session_id) DO NOTHING
	`, s.projectID, sessionID, ts, ts); err != nil {
		return false, err
	}

	var counter int
	if err := tx.QueryRow(`SELECT nudge_counter FROM sessions WHERE project_id = ? AND session_id = ?`, s.projectID, sessionID).Scan(&counter); err != nil {
		return false, err
	}
	counter++
	fire = counter >= threshold
	if fire {
		counter = 0
	}
	if _, err := tx.Exec(`UPDATE sessions SET nudge_counter = ? WHERE project_id = ? AND session_id = ?`, counter, s.projectID, sessionID); err != nil {
		return false, err
	}
	return fire, tx.Commit()
}

type Tip struct {
	ID      int64
	Message string
}

func (s *Store) AddTip(message string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO lg_tips (project_id, message, created_at) VALUES (?, ?, ?)`, s.projectID, message, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListTips() ([]Tip, error) {
	rows, err := s.db.Query(`SELECT id, message FROM lg_tips WHERE project_id = ? ORDER BY id`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tip
	for rows.Next() {
		var t Tip
		if err := rows.Scan(&t.ID, &t.Message); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTip(id int64) error {
	_, err := s.db.Exec(`DELETE FROM lg_tips WHERE project_id = ? AND id = ?`, s.projectID, id)
	return err
}

// Sign upserts, so re-signing before the previous TTL expires just resets the clock.
func (s *Store) Sign(sessionID, checklistID string) error {
	_, err := s.db.Exec(`
		INSERT INTO lg_signatures (project_id, session_id, checklist_id, signed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id, session_id, checklist_id) DO UPDATE SET signed_at = excluded.signed_at
	`, s.projectID, sessionID, checklistID, now())
	return err
}

// SignedAt returns the zero time, no error, when this session has never signed this checklist.
func (s *Store) SignedAt(sessionID, checklistID string) (time.Time, error) {
	var ts string
	err := s.db.QueryRow(`
		SELECT signed_at FROM lg_signatures WHERE project_id = ? AND session_id = ? AND checklist_id = ?
	`, s.projectID, sessionID, checklistID).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, ts)
}
