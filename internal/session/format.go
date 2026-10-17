package session

import (
	"fmt"
	"strings"
)

// The tool call is always allowed either way; this only ever adds visibility, never blocks.
func FormatCollisionWarning(hits []ActivityHit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("lgrass: recent activity from another session nearby -- check for overlap before proceeding:\n")
	for _, h := range hits {
		scope := "same folder"
		if h.SameFile {
			scope = "same file"
		}
		fmt.Fprintf(&b, "- session %s touched %s (%s)\n", h.SessionID, h.FilePath, scope)
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatNudge(liveness []SessionStatus) string {
	active := 0
	for _, s := range liveness {
		if s.Active {
			active++
		}
	}
	idling := len(liveness) - active

	var b strings.Builder
	fmt.Fprintf(&b, "lgrass: %d other session(s) open in this project", len(liveness))
	if len(liveness) > 0 {
		fmt.Fprintf(&b, " (%d active, %d idling)", active, idling)
	}
	return b.String()
}

const memoryFeedbackDenyLead = "lgrass: STOP. This call writes into Claude Code memory, which holds one-line pointers only. " +
	"A rule or feedback placed there is a violation and gets deleted; it goes in biblio/laws/. " +
	"Every write route is watched, whatever the tool. "

// This only formats the message, the caller owns the sign/TTL check.
func FormatMemoryFeedbackDeny() string {
	return memoryFeedbackDenyLead + "For a pointer, run `lgrass sign " + MemoryWriteWord + "` and retry."
}

func FormatMemoryHeldDeny(id string) string {
	return memoryFeedbackDenyLead + "The call did not run and is held as " + id + ". " +
		"Run `lgrass sign " + MemoryWriteWord + "`, check what it writes into memory, then run `lgrass replay " + id + "` instead of resending it."
}

func FormatTaskCloseDeny(slug string) string {
	return "lgrass: " + slug + " is being archived before its close instructions were read in this session. " +
		"Run `lgrass how task close " + slug + "`, follow its steps, then archive."
}

func FormatTempWriteDeny(target string) string {
	return "lgrass: writing " + target + " is blocked, because the system temp dir is wiped by a reboot or power cut. " +
		"In a bibliothek project every file the work produces goes in its task directory: task documents at biblio/scratchpad/<task-slug>/, " +
		"and scripts, downloads, dumps and command output in biblio/scratchpad/<task-slug>/dump/, each download in its own new directory there. " +
		"Reading from the temp dir is still allowed."
}

func FormatPlanModeDeny() string {
	return "lgrass: EnterPlanMode is banned in a bibliothek project. Plan the bibliothek way instead: work it out in biblio/scratchpad/<task-slug>/ (prd.md/plan.md), then present the plan directly in this response and wait for explicit confirmation before changing anything."
}
