package hook

import (
	"strings"

	"github.com/faizalv/lemongrass/internal/session"
)

// The collaborator this tab is in its live group, if any. Any lookup failure reads as not a collaborator, so a store problem never blocks a tool call.
func liveCollaborator(store *session.Store, tab string) (session.Member, session.Group, bool) {
	if tab == "" {
		return session.Member{}, session.Group{}, false
	}
	group, member, err := store.LiveMembership(tab)
	if err != nil || member.Role != session.RoleCollaborator {
		return session.Member{}, session.Group{}, false
	}
	return member, group, true
}

// collaboratorGateDeny holds a collaborator to lgrass commands until it has read its role with `lgrass how collaborator join` in this session, recording the mark when it does. Marks clear at every session start, so a compacted collaborator reads it again.
func collaboratorGateDeny(store *session.Store, payload hookEvent) string {
	if _, _, ok := liveCollaborator(store, payload.TabID); !ok {
		return ""
	}
	command := shellCommand(payload.ToolName, payload.ToolInput)
	if strings.Join(plainCommand(command), " ") == session.CollaboratorJoinCommand {
		store.MarkReady(payload.TabID, session.CollaboratorMark)
		return ""
	}
	if marks, err := store.Marks(payload.TabID); err != nil || marks[session.CollaboratorMark] {
		return ""
	}
	if isLgrassCommand(command) {
		return ""
	}
	return session.FormatCollaboratorDeny()
}
