// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"fmt"
	"strings"
	"time"
)

const shortTabLen = 8

// The display name of a tab outside any group.
func TabLabel(tabID string) string {
	if len(tabID) <= shortTabLen {
		return tabID
	}
	return tabID[:shortTabLen]
}

func labelOf(labels map[string]string, tabID string) string {
	if l, ok := labels[tabID]; ok {
		return l
	}
	return TabLabel(tabID)
}

func formatTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.UTC().Format("2006-01-02 15:04Z")
}

func formatDate(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.UTC().Format("2006-01-02")
}

// The clock time alone when the message is from the same day as the newest one shown, and the date with it otherwise.
func formatMessageTime(ts, newestTS string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	if formatDate(ts) == formatDate(newestTS) {
		return t.UTC().Format("15:04")
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// unread says the page holds only what the reader had not seen.
func FormatThreadRead(t Thread, msgs []Message, more, unread bool, labels map[string]string) string {
	var b strings.Builder
	scope := ""
	if unread {
		scope = ", unread only"
	}
	fmt.Fprintf(&b, "%s thread %s [%s], opened by %s %s, %d msgs%s, newest first, times UTC\n", Prefix, t.ID, t.Title, labelOf(labels, t.CreatedBy), formatDate(t.CreatedAt), t.MessageCount, scope)
	for _, m := range msgs {
		fmt.Fprintf(&b, "\n#%d %s %s", m.ID, formatMessageTime(m.CreatedAt, msgs[0].CreatedAt), labelOf(labels, m.TabID))
		if len(m.Mentions) > 0 {
			names := make([]string, len(m.Mentions))
			for i, id := range m.Mentions {
				names[i] = labelOf(labels, id)
			}
			fmt.Fprintf(&b, " (mentions %s)", strings.Join(names, ", "))
		}
		fmt.Fprintf(&b, ":\n%s\n", m.Body)
	}
	if more && len(msgs) > 0 {
		fmt.Fprintf(&b, "\nolder messages: lgrass thread read %s --before %d\n", t.ID, msgs[len(msgs)-1].ID)
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatThreadList(threads []Thread, labels map[string]string) string {
	if len(threads) == 0 {
		return "lgrass: no threads yet in this project."
	}
	var b strings.Builder
	for _, t := range threads {
		fmt.Fprintf(&b, "%s [%s] %d message(s), last activity %s, opened by %s\n", t.ID, t.Title, t.MessageCount, formatTime(t.LastActivityAt), labelOf(labels, t.CreatedBy))
	}
	return strings.TrimRight(b.String(), "\n")
}

// Marks text that comes from lemongrass or from other models, never from the human. The session start context and the thinker skill say so once.
const Prefix = "[lg]"

// One line per thread, one part per kind: what is for the reader, what is for everyone, and what is for someone else.
func FormatNotification(pending []PendingThread, labels map[string]string) string {
	lines := make([]string, len(pending))
	for i, p := range pending {
		var parts []string
		if p.You.Count > 0 {
			parts = append(parts, fmt.Sprintf("%d for you from %s", p.You.Count, labelList(labels, p.You.Tabs)))
		}
		if p.All.Count > 0 {
			parts = append(parts, fmt.Sprintf("%d new from %s", p.All.Count, labelList(labels, p.All.Tabs)))
		}
		if p.Other.Count > 0 {
			parts = append(parts, fmt.Sprintf("%d for %s, not you", p.Other.Count, labelList(labels, p.Other.Tabs)))
		}
		lines[i] = fmt.Sprintf("%s thread %s: %s", Prefix, p.ThreadID, strings.Join(parts, "; "))
	}
	return strings.Join(lines, "\n")
}

func labelList(labels map[string]string, ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = labelOf(labels, id)
	}
	return strings.Join(names, ", ")
}

// Store-aware wrapper: resolves the senders' labels before formatting.
func (s *Store) NotificationText(pending []PendingThread) string {
	var ids []string
	for _, p := range pending {
		ids = append(ids, p.You.Tabs...)
		ids = append(ids, p.All.Tabs...)
		ids = append(ids, p.Other.Tabs...)
	}
	return FormatNotification(pending, s.Labels(ids))
}

func FormatGroupHeader(g Group, members []Member) string {
	state := "live"
	if !g.Live() {
		state = "disbanded " + formatTime(g.DisbandedAt)
	}
	parts := make([]string, len(members))
	for i, m := range members {
		parts[i] = fmt.Sprintf("%s (%s, %s, tab %s)", m.Label, m.Role, m.Vendor, TabLabel(m.TabID))
	}
	return fmt.Sprintf("lgrass workgroup %d [%s], thread %s, %s, members: %s", g.ID, g.Name, g.ThreadID, state, strings.Join(parts, "; "))
}

// The repeat visit to a group thread: the full header is already in the reader's context.
func FormatGroupShort(g Group, m Member) string {
	return fmt.Sprintf("%s workgroup %d [%s], thread %s, you are %s (%s). lgrass workgroup thread --all repeats the members.", Prefix, g.ID, g.Name, g.ThreadID, m.Label, m.Role)
}

func FormatNothingNew(t Thread) string {
	return fmt.Sprintf("%s thread %s [%s]: nothing new. lgrass thread read %s --all shows the latest messages.", Prefix, t.ID, t.Title, t.ID)
}

func FormatThinkerStart(m Member, group Group) string {
	text := fmt.Sprintf("%s you are thinker %q in workgroup %q. Load these skills first: %s. Then run lgrass workgroup thread.",
		Prefix, m.Label, group.Name, strings.Join(m.RequiredSkills(), ", "))
	return text
}

// CollaboratorJoinCommand is the read that teaches a collaborator its role; the collaborator gate opens once a tab has run it.
const CollaboratorJoinCommand = "lgrass how collaborator join"

func FormatCollaboratorStart(m Member, group Group) string {
	return fmt.Sprintf("%s you are collaborator %q in workgroup %q. Run %s first, then lgrass workgroup thread.", Prefix, m.Label, group.Name, CollaboratorJoinCommand)
}

func FormatCollaboratorDeny() string {
	return fmt.Sprintf("%s you are a collaborator in a workgroup. Run `%s` first. Until then only lgrass commands are allowed.", Prefix, CollaboratorJoinCommand)
}

func FormatThinkerSkillsDeny(missing []string) string {
	return fmt.Sprintf("%s load these skills first: %s. Claude Code: the Skill tool. Others: read the skill's SKILL.md. Until then only lgrass commands and those loads are allowed.", Prefix, strings.Join(missing, ", "))
}

// The one line that teaches what the prefix means, added to the start context of every lemongrass tab.
func FormatPrefixNote() string {
	return Prefix + " marks text from lemongrass or other models, never your user. lgrass thread read <id> reads a thread. lgrass session list shows tabs."
}

// What the caller is in its group, with its assignment and required skills, so a resumed thinker gets its role back.
func FormatMemberHeader(m Member) string {
	if m.Role == RoleLeader {
		return "You are the leader of this workgroup. Load the lgrass-howtobe-leader skill if you have not."
	}
	if m.Role == RoleCollaborator {
		return fmt.Sprintf("You are the collaborator %q. Run `%s` if you have not in this session.", m.Label, CollaboratorJoinCommand)
	}
	text := fmt.Sprintf("You are the thinker %q. Skills you must load: %s.", m.Label, strings.Join(m.RequiredSkills(), ", "))
	if m.Prompt != "" {
		text += "\nYour assignment:\n" + m.Prompt
	}
	return text
}
