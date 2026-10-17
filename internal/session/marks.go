package session

// A mark that a tab has loaded a skill.
func SkillMark(skill string) string { return "skill:" + skill }

// A mark that a tab has read the close instructions for a scratchpad task.
func TaskCloseMark(slug string) string { return "task-close:" + slug }

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

func (s *Store) ClearMarks(tabID string) error {
	_, err := s.db.Exec(`DELETE FROM lg_ready_marks WHERE project_id = ? AND tab_id = ?`, s.projectID, tabID)
	return err
}
