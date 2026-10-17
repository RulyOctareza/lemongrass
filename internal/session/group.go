package session

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	RoleLeader  = "leader"
	RoleThinker = "thinker"

	// Every thinker loads this skill.
	ThinkerSkill = "lgrass-howtobe-thinker"
	// BibliothekSkill is never required of a Claude thinker, whose system prompt already carries it.
	BibliothekSkill = "bibliothek"
)

type Group struct {
	ID          int64
	LeaderTabID string
	ThreadID    string
	Name        string
	CreatedAt   string
	DisbandedAt string // empty while the group is live
}

func (g Group) Live() bool { return g.DisbandedAt == "" }

type Member struct {
	TabID  string
	Role   string
	Label  string
	Vendor string
	Prompt string   // the assignment from the group config
	Skills []string // skills the member must load, beyond the ones every thinker loads
}

var (
	ErrNoSuchGroup   = errors.New("session: no such workgroup in this project")
	ErrAlreadyInside = errors.New("session: a tab can belong to only one live workgroup")
	ErrNameTaken     = errors.New("session: a workgroup already has this name")
	ErrLabelTaken    = errors.New("session: a member of this workgroup already has this label")
)

type rowQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func groupNameTaken(q rowQuerier, name string) (bool, error) {
	rows, err := q.Query(`SELECT title FROM lg_threads WHERE group_id IS NOT NULL`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	want := strings.TrimSpace(name)
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(title), want) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Names are unique across every project's workgroups, live or disbanded, ignoring case and surrounding spaces.
func (s *Store) CheckGroupName(name string) error {
	taken, err := groupNameTaken(s.db, name)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w (%q)", ErrNameTaken, strings.TrimSpace(name))
	}
	return nil
}

// Creates the group, its members, and its one thread in a single transaction. The leader is added as a member.
func (s *Store) CreateGroup(name string, leader Member, thinkers []Member) (Group, error) {
	leader.Role = RoleLeader
	members := []Member{leader}
	for _, c := range thinkers {
		c.Role = RoleThinker
		members = append(members, c)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Group{}, err
	}
	defer tx.Rollback()

	taken, err := groupNameTaken(tx, name)
	if err != nil {
		return Group{}, err
	}
	if taken {
		return Group{}, fmt.Errorf("%w (%q)", ErrNameTaken, strings.TrimSpace(name))
	}

	for _, m := range members {
		var one int
		err := tx.QueryRow(`
			SELECT 1 FROM lg_group_members m JOIN lg_groups g ON g.id = m.group_id
			WHERE m.tab_id = ? AND g.project_id = ? AND g.disbanded_at IS NULL
		`, m.TabID, s.projectID).Scan(&one)
		if err == nil {
			return Group{}, fmt.Errorf("%w (%s)", ErrAlreadyInside, TabLabel(m.TabID))
		}
		if err != sql.ErrNoRows {
			return Group{}, err
		}
	}

	ts := now()
	res, err := tx.Exec(`INSERT INTO lg_groups (project_id, leader_tab_id, created_at) VALUES (?, ?, ?)`, s.projectID, leader.TabID, ts)
	if err != nil {
		return Group{}, err
	}
	groupID, err := res.LastInsertId()
	if err != nil {
		return Group{}, err
	}
	for _, m := range members {
		if _, err := tx.Exec(`INSERT INTO lg_group_members (group_id, tab_id, role, label, vendor, prompt, skills) VALUES (?, ?, ?, ?, ?, ?, ?)`, groupID, m.TabID, m.Role, m.Label, m.Vendor, m.Prompt, strings.Join(m.Skills, ",")); err != nil {
			return Group{}, err
		}
	}
	threadID, err := newThreadID(tx)
	if err != nil {
		return Group{}, err
	}
	if _, err := tx.Exec(`INSERT INTO lg_threads (id, project_id, title, group_id, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, threadID, s.projectID, name, groupID, leader.TabID, ts); err != nil {
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return Group{ID: groupID, LeaderTabID: leader.TabID, ThreadID: threadID, Name: name, CreatedAt: ts}, nil
}

const groupSelect = `
	SELECT g.id, g.leader_tab_id, COALESCE(t.id, ''), COALESCE(t.title, ''), g.created_at, COALESCE(g.disbanded_at, '')
	FROM lg_groups g LEFT JOIN lg_threads t ON t.group_id = g.id
`

func scanGroup(row *sql.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.LeaderTabID, &g.ThreadID, &g.Name, &g.CreatedAt, &g.DisbandedAt)
	if err == sql.ErrNoRows {
		return Group{}, ErrNoSuchGroup
	}
	return g, err
}

func (s *Store) GroupByID(id int64) (Group, error) {
	return scanGroup(s.db.QueryRow(groupSelect+` WHERE g.project_id = ? AND g.id = ?`, s.projectID, id))
}

// The live group the tab belongs to as leader or member; ErrNoSuchGroup when it has none.
func (s *Store) LiveGroupForTab(tabID string) (Group, error) {
	return scanGroup(s.db.QueryRow(groupSelect+`
		JOIN lg_group_members m ON m.group_id = g.id
		WHERE g.project_id = ? AND g.disbanded_at IS NULL AND m.tab_id = ?
	`, s.projectID, tabID))
}

func (s *Store) LiveGroups() ([]Group, error) {
	rows, err := s.db.Query(groupSelect+` WHERE g.project_id = ? AND g.disbanded_at IS NULL ORDER BY g.id`, s.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.LeaderTabID, &g.ThreadID, &g.Name, &g.CreatedAt, &g.DisbandedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GroupMembers(groupID int64) ([]Member, error) {
	rows, err := s.db.Query(`SELECT tab_id, role, label, vendor, prompt, skills FROM lg_group_members WHERE group_id = ? ORDER BY CASE role WHEN 'leader' THEN 0 ELSE 1 END, rowid`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var skills string
		if err := rows.Scan(&m.TabID, &m.Role, &m.Label, &m.Vendor, &m.Prompt, &skills); err != nil {
			return nil, err
		}
		if skills != "" {
			m.Skills = strings.Split(skills, ",")
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// The tab's live group and its own member row; ErrNoSuchGroup when the tab is in no live group.
func (s *Store) LiveMembership(tabID string) (Group, Member, error) {
	group, err := s.LiveGroupForTab(tabID)
	if err != nil {
		return Group{}, Member{}, err
	}
	members, err := s.GroupMembers(group.ID)
	if err != nil {
		return Group{}, Member{}, err
	}
	for _, m := range members {
		if m.TabID == tabID {
			return group, m, nil
		}
	}
	return Group{}, Member{}, ErrNoSuchGroup
}

// AddMembers joins thinkers to a live group in one transaction. A label is unique within the group ignoring case, and a tab can be in only one live group.
func (s *Store) AddMembers(groupID int64, thinkers []Member) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var disbanded sql.NullString
	err = tx.QueryRow(`SELECT disbanded_at FROM lg_groups WHERE id = ? AND project_id = ?`, groupID, s.projectID).Scan(&disbanded)
	if err == sql.ErrNoRows {
		return ErrNoSuchGroup
	}
	if err != nil {
		return err
	}
	if disbanded.Valid {
		return fmt.Errorf("session: workgroup %d is disbanded", groupID)
	}

	rows, err := tx.Query(`SELECT label FROM lg_group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return err
	}
	labels := map[string]bool{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			rows.Close()
			return err
		}
		labels[strings.ToLower(label)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range thinkers {
		if labels[strings.ToLower(m.Label)] {
			return fmt.Errorf("%w (%q)", ErrLabelTaken, m.Label)
		}
		labels[strings.ToLower(m.Label)] = true
		var one int
		err := tx.QueryRow(`
			SELECT 1 FROM lg_group_members m JOIN lg_groups g ON g.id = m.group_id
			WHERE m.tab_id = ? AND g.project_id = ? AND g.disbanded_at IS NULL
		`, m.TabID, s.projectID).Scan(&one)
		if err == nil {
			return fmt.Errorf("%w (%s)", ErrAlreadyInside, TabLabel(m.TabID))
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO lg_group_members (group_id, tab_id, role, label, vendor, prompt, skills) VALUES (?, ?, ?, ?, ?, ?, ?)`, groupID, m.TabID, RoleThinker, m.Label, m.Vendor, m.Prompt, strings.Join(m.Skills, ",")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Removes members whose tabs never opened, so a failed spawn leaves the group as it was.
func (s *Store) DeleteMembers(groupID int64, tabIDs []string) error {
	for _, tab := range tabIDs {
		if _, err := s.db.Exec(`DELETE FROM lg_group_members WHERE group_id = ? AND tab_id = ? AND role = ? AND group_id IN (SELECT id FROM lg_groups WHERE project_id = ?)`, groupID, tab, RoleThinker, s.projectID); err != nil {
			return err
		}
	}
	return nil
}

// Keeps the thread and its messages, so a disbanded group stays readable.
func (s *Store) DisbandGroup(id int64) error {
	res, err := s.db.Exec(`UPDATE lg_groups SET disbanded_at = ? WHERE project_id = ? AND id = ? AND disbanded_at IS NULL`, now(), s.projectID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GroupByID(id); err != nil {
			return err
		}
		return fmt.Errorf("session: workgroup %d is already disbanded", id)
	}
	return nil
}

// Removes a group that never got going, with its members and its empty thread.
func (s *Store) DeleteGroup(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM lg_threads WHERE group_id = ? AND project_id = ? AND NOT EXISTS (SELECT 1 FROM lg_messages WHERE thread_id = lg_threads.id)`, id, s.projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM lg_group_members WHERE group_id = ? AND group_id IN (SELECT id FROM lg_groups WHERE project_id = ?)`, id, s.projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM lg_groups WHERE id = ? AND project_id = ?`, id, s.projectID); err != nil {
		return err
	}
	return tx.Commit()
}

// Maps every requested tab id to its group label, taken from its most recent membership in this project, or to its short id when it has none.
func (s *Store) Labels(tabIDs []string) map[string]string {
	out := make(map[string]string, len(tabIDs))
	if len(tabIDs) == 0 {
		return out
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(tabIDs)), ",")
	args := []interface{}{s.projectID}
	for _, id := range tabIDs {
		out[id] = TabLabel(id)
		args = append(args, id)
	}
	rows, err := s.db.Query(`
		SELECT m.tab_id, m.label FROM lg_group_members m JOIN lg_groups g ON g.id = m.group_id
		WHERE g.project_id = ? AND m.tab_id IN (`+marks+`) ORDER BY g.id
	`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, label string
		if rows.Scan(&id, &label) == nil {
			out[id] = label
		}
	}
	return out
}

// The thinker skill first, then the skills the leader listed, each once.
func (m Member) RequiredSkills() []string {
	out := []string{ThinkerSkill}
	for _, name := range m.Skills {
		if !containsString(out, name) && !CarriedInPrompt(m.Vendor, name) {
			out = append(out, name)
		}
	}
	return out
}

func CarriedInPrompt(vendor, skill string) bool {
	return vendor == "claude" && skill == BibliothekSkill
}
