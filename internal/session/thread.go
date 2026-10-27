// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxMessageRunes = 2000
	MaxTitleRunes   = 120
)

var ErrNoSuchThread = errors.New("session: no such thread in this project")

type Thread struct {
	ID             string
	Title          string
	GroupID        int64 // 0 for an ad hoc thread
	CreatedBy      string
	CreatedAt      string
	LastActivityAt string
	MessageCount   int
}

type Message struct {
	ID        int64
	ThreadID  string
	TabID     string
	Body      string
	CreatedAt string
	Mentions  []string
}

func validateMessage(body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("session: message is empty")
	}
	if n := utf8.RuneCountInString(body); n > MaxMessageRunes {
		return fmt.Errorf("session: message is %d characters and the cap is %d, put the long content in a scratchpad note and post a pointer to it", n, MaxMessageRunes)
	}
	return nil
}

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("session: thread title is empty")
	}
	if n := utf8.RuneCountInString(title); n > MaxTitleRunes {
		return fmt.Errorf("session: thread title is %d characters and the cap is %d", n, MaxTitleRunes)
	}
	return nil
}

// Opens an ad hoc thread, whose first message is content.
func (s *Store) CreateThread(tabID, title, content string) (string, error) {
	title = strings.TrimSpace(title)
	if err := validateTitle(title); err != nil {
		return "", err
	}
	if err := validateMessage(content); err != nil {
		return "", err
	}
	mentions, err := s.ResolveMentions(ParseMentions(content))
	if err != nil {
		return "", err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	ts := now()
	threadID, err := newThreadID(tx)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO lg_threads (id, project_id, title, created_by, created_at) VALUES (?, ?, ?, ?, ?)`, threadID, s.projectID, title, tabID, ts); err != nil {
		return "", err
	}
	if _, err := insertMessage(tx, s.projectID, threadID, tabID, content, mentions, mentions, ts); err != nil {
		return "", err
	}
	return threadID, tx.Commit()
}

// SystemAuthor stands in for a tab id on messages lemongrass posts itself, so no member is the author and every member is notified.
const SystemAuthor = "lgrass"

// Posts a lemongrass message to a live group's thread for every member, with no mentions.
func (s *Store) PostSystemMessage(threadID string, content string) (Message, error) {
	if err := validateMessage(content); err != nil {
		return Message{}, err
	}
	thread, err := s.ThreadByID(threadID)
	if err != nil {
		return Message{}, err
	}
	if thread.GroupID == 0 {
		return Message{}, fmt.Errorf("session: thread %s is not a workgroup thread", threadID)
	}
	group, err := s.GroupByID(thread.GroupID)
	if err != nil {
		return Message{}, err
	}
	if !group.Live() {
		return Message{}, fmt.Errorf("session: workgroup %d is disbanded", group.ID)
	}
	members, err := s.GroupMembers(group.ID)
	if err != nil {
		return Message{}, err
	}
	notify := make([]string, len(members))
	for i, m := range members {
		notify[i] = m.TabID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()

	msg, err := insertMessage(tx, s.projectID, threadID, SystemAuthor, content, nil, notify, now())
	if err != nil {
		return Message{}, err
	}
	return msg, tx.Commit()
}

func (s *Store) PostMessage(tabID string, threadID string, content string) (Message, error) {
	if err := validateMessage(content); err != nil {
		return Message{}, err
	}
	thread, err := s.ThreadByID(threadID)
	if err != nil {
		return Message{}, err
	}
	mentions, err := s.ResolveMentions(ParseMentions(content))
	if err != nil {
		return Message{}, err
	}
	var notify []string
	if thread.GroupID != 0 {
		notify, err = s.groupRecipients(thread.GroupID, tabID, mentions)
	} else {
		notify, err = s.adHocRecipients(threadID, mentions)
	}
	if err != nil {
		return Message{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()

	msg, err := insertMessage(tx, s.projectID, threadID, tabID, content, mentions, notify, now())
	if err != nil {
		return Message{}, err
	}
	return msg, tx.Commit()
}

// How a target relates to a message: mentioned, addressed to everyone, or addressed to someone else.
func notificationKind(target string, mentions []string) string {
	switch {
	case containsString(mentions, target):
		return KindYou
	case len(mentions) == 0:
		return KindAll
	default:
		return KindOther
	}
}

// Every tab in notify except the author gets a pending notification, and mentions are recorded on the message.
func insertMessage(tx *sql.Tx, projectID string, threadID string, tabID, body string, mentions, notify []string, ts string) (Message, error) {
	res, err := tx.Exec(`INSERT INTO lg_messages (thread_id, tab_id, body, created_at) VALUES (?, ?, ?, ?)`, threadID, tabID, body, ts)
	if err != nil {
		return Message{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Message{}, err
	}
	for _, tab := range mentions {
		if _, err := tx.Exec(`INSERT INTO lg_message_mentions (message_id, tab_id) VALUES (?, ?)`, id, tab); err != nil {
			return Message{}, err
		}
	}
	for _, tab := range notify {
		if tab == tabID {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO lg_notifications (project_id, source, ref, target_tab_id, kind, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, projectID, SourceThread, strconv.FormatInt(id, 10), tab, notificationKind(tab, mentions), ts); err != nil {
			return Message{}, err
		}
	}
	return Message{ID: id, ThreadID: threadID, TabID: tabID, Body: body, CreatedAt: ts, Mentions: mentions}, nil
}

// An ad hoc thread notifies the mentioned tabs plus every registered tab that has posted to it or been mentioned in it, so a reply reaches the tab it answers without a mention.
func (s *Store) adHocRecipients(threadID string, mentions []string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT tab_id FROM lg_messages WHERE thread_id = ?
		UNION
		SELECT mm.tab_id FROM lg_message_mentions mm JOIN lg_messages m ON m.id = mm.message_id WHERE m.thread_id = ?
	`, threadID, threadID)
	if err != nil {
		return nil, err
	}
	var participants []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		participants = append(participants, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	vendors, err := s.TabVendors()
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), mentions...)
	for _, id := range participants {
		if _, live := vendors[id]; live && !containsString(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// A group thread accepts posts from live members only, and notifies every member plus the mentioned tabs.
func (s *Store) groupRecipients(groupID int64, author string, mentions []string) ([]string, error) {
	group, err := s.GroupByID(groupID)
	if err != nil {
		return nil, err
	}
	if !group.Live() {
		return nil, fmt.Errorf("session: workgroup %d is disbanded", group.ID)
	}
	members, err := s.GroupMembers(groupID)
	if err != nil {
		return nil, err
	}
	isMember := false
	out := append([]string(nil), mentions...)
	for _, m := range members {
		if m.TabID == author {
			isMember = true
		}
		if !containsString(out, m.TabID) {
			out = append(out, m.TabID)
		}
	}
	if !isMember {
		return nil, fmt.Errorf("session: only members of workgroup %d can post to its thread", groupID)
	}
	return out, nil
}

func (s *Store) ThreadByID(id string) (Thread, error) {
	var t Thread
	var group sql.NullInt64
	err := s.db.QueryRow(`
		SELECT t.id, t.title, t.group_id, t.created_by, t.created_at,
			COALESCE((SELECT created_at FROM lg_messages WHERE thread_id = t.id ORDER BY id DESC LIMIT 1), t.created_at),
			(SELECT COUNT(*) FROM lg_messages WHERE thread_id = t.id)
		FROM lg_threads t WHERE t.project_id = ? AND t.id = ?
	`, s.projectID, id).Scan(&t.ID, &t.Title, &group, &t.CreatedBy, &t.CreatedAt, &t.LastActivityAt, &t.MessageCount)
	if err == sql.ErrNoRows {
		return Thread{}, ErrNoSuchThread
	}
	if err != nil {
		return Thread{}, err
	}
	t.GroupID = group.Int64
	return t, nil
}

// Newest first, at most limit messages older than beforeID (0 for the newest page); more reports whether older ones remain.
func (s *Store) ReadThread(threadID string, beforeID int64, limit int) (msgs []Message, more bool, err error) {
	return s.readMessages(threadID, beforeID, 0, "", limit)
}

// Newest first, at most limit messages newer than afterID that tabID did not write; more reports whether older ones remain, read or not.
func (s *Store) ReadUnread(threadID string, tabID string, afterID int64, limit int) (msgs []Message, more bool, err error) {
	return s.readMessages(threadID, 0, afterID, tabID, limit)
}

func (s *Store) readMessages(threadID string, beforeID, afterID int64, excludeTab string, limit int) (msgs []Message, more bool, err error) {
	if limit < 1 {
		limit = 1
	}
	query := `SELECT id, thread_id, tab_id, body, created_at FROM lg_messages WHERE thread_id = ?`
	args := []interface{}{threadID}
	if beforeID > 0 {
		query += ` AND id < ?`
		args = append(args, beforeID)
	}
	if afterID > 0 {
		query += ` AND id > ?`
		args = append(args, afterID)
	}
	if excludeTab != "" {
		query += ` AND tab_id != ?`
		args = append(args, excludeTab)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.TabID, &m.Body, &m.CreatedAt); err != nil {
			return nil, false, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(msgs) > 0 {
		var older int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT 1 FROM lg_messages WHERE thread_id = ? AND id < ? LIMIT 1)`, threadID, msgs[len(msgs)-1].ID).Scan(&older); err != nil {
			return nil, false, err
		}
		more = older > 0
	}
	for i := range msgs {
		if msgs[i].Mentions, err = s.messageMentions(msgs[i].ID); err != nil {
			return nil, false, err
		}
	}
	return msgs, more, nil
}

func (s *Store) messageMentions(messageID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT tab_id FROM lg_message_mentions WHERE message_id = ? ORDER BY tab_id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Most recently active first.
func (s *Store) ListThreads(limit int) ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.title, t.group_id, t.created_by, t.created_at,
			COALESCE(MAX(m.created_at), t.created_at), COUNT(m.id)
		FROM lg_threads t LEFT JOIN lg_messages m ON m.thread_id = t.id
		WHERE t.project_id = ?
		GROUP BY t.id
		ORDER BY COALESCE(MAX(m.id), 0) DESC, t.id DESC
		LIMIT ?
	`, s.projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var group sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Title, &group, &t.CreatedBy, &t.CreatedAt, &t.LastActivityAt, &t.MessageCount); err != nil {
			return nil, err
		}
		t.GroupID = group.Int64
		out = append(out, t)
	}
	return out, rows.Err()
}
