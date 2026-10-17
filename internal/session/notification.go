package session

import (
	"database/sql"
	"strings"
	"time"
)

// Kept after being read so the ledger doubles as an audit trail.
const NotificationRetention = 30 * 24 * time.Hour

const (
	stateSent = "sent"
	stateRead = "read"
)

// How a notification's target relates to its message.
const (
	KindYou   = "you"   // the target is mentioned
	KindAll   = "all"   // the message mentions nobody, so it is for everyone
	KindOther = "other" // the message mentions someone else and not the target
)

// The notifications of one kind for one thread. Tabs are the authors for you and all, and the mentioned tabs for other.
type KindPart struct {
	Count int
	Tabs  []string
}

func (k *KindPart) add(tab string) {
	k.Count++
	if !containsString(k.Tabs, tab) {
		k.Tabs = append(k.Tabs, tab)
	}
}

// The pending notifications of one tab for one thread.
type PendingThread struct {
	ThreadID string
	Title    string
	You      KindPart
	All      KindPart
	Other    KindPart
	RowIDs   []int64
}

func (p PendingThread) Count() int { return len(p.RowIDs) }

// The rows that may wake a tab, meaning every kind but other. A tab id is globally unique, so this ignores the store's project.
func (s *Store) PendingForTab(tabID string) ([]PendingThread, error) {
	return s.pendingRows(tabID, `AND n.kind != 'other'`)
}

// Every pending row of the tab, other included, for a tab that is already mid-turn and sees them as hook context.
func (s *Store) SurfaceableForTab(tabID string) ([]PendingThread, error) {
	return s.pendingRows(tabID, ``)
}

func (s *Store) pendingRows(tabID, filter string) ([]PendingThread, error) {
	rows, err := s.db.Query(`
		SELECT n.id, n.thread_id, t.title, m.tab_id, n.kind, n.message_id
		FROM lg_notifications n
		JOIN lg_threads t ON t.id = n.thread_id
		JOIN lg_messages m ON m.id = n.message_id
		WHERE n.target_tab_id = ? AND n.state = 'pending' `+filter+`
		ORDER BY n.id
	`, tabID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id, messageID       int64
		threadID            string
		title, author, kind string
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.threadID, &r.title, &r.author, &r.kind, &r.messageID); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []PendingThread
	index := map[string]int{}
	for _, r := range found {
		i, ok := index[r.threadID]
		if !ok {
			i = len(out)
			index[r.threadID] = i
			out = append(out, PendingThread{ThreadID: r.threadID, Title: r.title})
		}
		out[i].RowIDs = append(out[i].RowIDs, r.id)
		switch r.kind {
		case KindYou:
			out[i].You.add(r.author)
		case KindOther:
			mentioned, err := s.messageMentions(r.messageID)
			if err != nil {
				return nil, err
			}
			out[i].Other.Count++
			for _, tab := range mentioned {
				if !containsString(out[i].Other.Tabs, tab) {
					out[i].Other.Tabs = append(out[i].Other.Tabs, tab)
				}
			}
		default:
			out[i].All.add(r.author)
		}
	}
	return out, nil
}

// The tab's pending other rows are marked sent when a nudge line already told it to read the thread.
func (s *Store) MarkOtherSent(tabID string) error {
	_, err := s.db.Exec(`UPDATE lg_notifications SET state = 'sent', sent_at = ? WHERE target_tab_id = ? AND kind = 'other' AND state = 'pending'`, now(), tabID)
	return err
}

// The newest message the tab has seen in the thread; false when it never read it.
func (s *Store) ThreadCursor(tabID string, threadID string) (int64, bool, error) {
	var last int64
	err := s.db.QueryRow(`SELECT last_message_id FROM lg_thread_reads WHERE tab_id = ? AND thread_id = ?`, tabID, threadID).Scan(&last)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return last, err == nil, err
}

// Only moves forward.
func (s *Store) AdvanceThreadCursor(tabID, threadID string, messageID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO lg_thread_reads (tab_id, thread_id, last_message_id) VALUES (?, ?, ?)
		ON CONFLICT (tab_id, thread_id) DO UPDATE SET last_message_id = MAX(last_message_id, excluded.last_message_id)
	`, tabID, threadID, messageID)
	return err
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func idPlaceholders(ids []int64) (string, []interface{}) {
	marks := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ","), args
}

func (s *Store) MarkNotificationsSent(rowIDs []int64) error {
	if len(rowIDs) == 0 {
		return nil
	}
	marks, args := idPlaceholders(rowIDs)
	_, err := s.db.Exec(`UPDATE lg_notifications SET state = 'sent', sent_at = ?, attempts = attempts + 1, last_attempt_at = ? WHERE state = 'pending' AND id IN (`+marks+`)`, append([]interface{}{now(), now()}, args...)...)
	return err
}

func (s *Store) RecordNotificationAttempt(rowIDs []int64) error {
	if len(rowIDs) == 0 {
		return nil
	}
	marks, args := idPlaceholders(rowIDs)
	_, err := s.db.Exec(`UPDATE lg_notifications SET attempts = attempts + 1, last_attempt_at = ? WHERE id IN (`+marks+`)`, append([]interface{}{now()}, args...)...)
	return err
}

// Settles the tab's rows for exactly the messages it read.
func (s *Store) MarkMessagesRead(tabID string, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	marks, args := idPlaceholders(messageIDs)
	_, err := s.db.Exec(`UPDATE lg_notifications SET state = 'read', read_at = ? WHERE target_tab_id = ? AND state != 'read' AND message_id IN (`+marks+`)`, append([]interface{}{now(), tabID}, args...)...)
	return err
}

// Tabs that have pending rows that may wake them, whatever their vendor.
func (s *Store) PendingTabs() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT target_tab_id FROM lg_notifications WHERE state = 'pending' AND kind != 'other'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tab string
		if err := rows.Scan(&tab); err != nil {
			return nil, err
		}
		out = append(out, tab)
	}
	return out, rows.Err()
}

// Tabs with a pending row that has attempts left and whose backoff since the last attempt has elapsed.
func (s *Store) TabsDueForRetry(maxAttempts int, backoff func(attempts int) time.Duration, at time.Time) ([]string, error) {
	rows, err := s.db.Query(`SELECT target_tab_id, attempts, last_attempt_at FROM lg_notifications WHERE state = 'pending' AND kind != 'other' AND attempts < ?`, maxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []string
	for rows.Next() {
		var tab string
		var attempts int
		var last sql.NullString
		if err := rows.Scan(&tab, &attempts, &last); err != nil {
			return nil, err
		}
		if seen[tab] {
			continue
		}
		due := true
		if last.Valid && attempts > 0 {
			if t, err := time.Parse(time.RFC3339Nano, last.String); err == nil {
				due = !at.Before(t.Add(backoff(attempts)))
			}
		}
		if due {
			seen[tab] = true
			out = append(out, tab)
		}
	}
	return out, rows.Err()
}

func (s *Store) PruneNotifications(at time.Time) error {
	cutoff := at.Add(-NotificationRetention).UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`DELETE FROM lg_notifications WHERE created_at < ?`, cutoff)
	return err
}

// Empty string, no error, when neither a tab record nor a group membership names the tab.
func (s *Store) VendorForTab(tabID string) (string, error) {
	var vendor string
	err := s.db.QueryRow(`SELECT vendor FROM lg_tabs WHERE tab_id = ? LIMIT 1`, tabID).Scan(&vendor)
	if err == nil {
		return vendor, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = s.db.QueryRow(`SELECT vendor FROM lg_group_members WHERE tab_id = ? ORDER BY group_id DESC LIMIT 1`, tabID).Scan(&vendor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return vendor, err
}

// The tab's pending nudge text with its rows marked sent; false when nothing is pending.
func (s *Store) ConsumePending(tab string) (string, bool) {
	pending, err := s.PendingForTab(tab)
	if err != nil || len(pending) == 0 {
		return "", false
	}
	var rowIDs []int64
	for _, p := range pending {
		rowIDs = append(rowIDs, p.RowIDs...)
	}
	s.MarkNotificationsSent(rowIDs)
	s.MarkOtherSent(tab)
	return s.NotificationText(pending), true
}
