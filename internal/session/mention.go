package session

import (
	"fmt"
	"regexp"
	"strings"
)

// The shortest tab id prefix a mention may use.
const minMentionPrefix = 8

// A tab or session id cut to the shortest tab id prefix a mention accepts.
func ShortID(id string) string {
	if len(id) > minMentionPrefix {
		return id[:minMentionPrefix]
	}
	return id
}

// Only the opening !>> marks a mention, because models often drop part of the closing <<!, so the closing may be <<!, <<, <!, < or absent.
var (
	mentionMarker = regexp.MustCompile(`!>>([\w-]*)(?:<<?!?)?`)
	tabIDPrefix   = regexp.MustCompile(`^[0-9a-fA-F-]+$`)
)

// Distinct tokens after each !>> in order of first appearance; a token that is not a tab id is kept so the post is rejected.
func ParseMentions(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mentionMarker.FindAllStringSubmatch(body, -1) {
		token := m[1]
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

// Resolves each token to exactly one tab id known in this project, either registered or a group member.
func (s *Store) ResolveMentions(tokens []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, token := range tokens {
		if len(token) < minMentionPrefix || !tabIDPrefix.MatchString(token) {
			return nil, fmt.Errorf("session: mention %q is not a tab id or a prefix of at least %d characters of one", token, minMentionPrefix)
		}
		prefix := strings.ToLower(token)
		rows, err := s.db.Query(`
			SELECT tab_id FROM lg_tabs WHERE project_id = ? AND substr(tab_id, 1, ?) = ?
			UNION
			SELECT m.tab_id FROM lg_group_members m JOIN lg_groups g ON g.id = m.group_id
			WHERE g.project_id = ? AND substr(m.tab_id, 1, ?) = ?
		`, s.projectID, len(prefix), prefix, s.projectID, len(prefix), prefix)
		if err != nil {
			return nil, err
		}
		var matches []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			matches = append(matches, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("session: mention %q matches no tab in this project, see `lgrass session list`", token)
		case 1:
			if !seen[matches[0]] {
				seen[matches[0]] = true
				out = append(out, matches[0])
			}
		default:
			return nil, fmt.Errorf("session: mention %q matches %d tabs, use a longer prefix", token, len(matches))
		}
	}
	return out, nil
}
