// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/session"
)

const (
	collisionWindow  = 15 * time.Minute
	idleThreshold    = 5 * time.Minute
	nudgeThreshold   = 8 // tool calls between periodic nudges
	hookProbeLogPath = "/tmp/lgrass-hook-probe.log"
	hookProbeEnv     = "LGRASS_HOOK_PROBE"
	lawsInPromptEnv  = "LGRASS_LAWS_IN_PROMPT"
)

// The full hook JSON carries more fields depending on hook_event_name; this is only the subset lgrass reads.
type hookEvent struct {
	SessionID            string          `json:"session_id"`
	TranscriptPath       string          `json:"transcript_path"`
	Cwd                  string          `json:"cwd"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	SessionSource        string          `json:"source"`
	NotificationType     string          `json:"notification_type"`
	Message              string          `json:"message"`
	TabID                string
	EnforceBibliothek    bool
	Raw                  []byte `json:"-"`
	PreserveSessionState bool
}

type fileToolInput struct {
	FilePath string `json:"file_path"`
}

type patchToolInput struct {
	Command string `json:"command"`
}

// Every failure here fails soft so a hook-side problem never breaks the agent's own hook chain.
func Run(args []string, tabID string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lgrassd hook <SessionStart|SessionEnd|PreToolUse|PostToolUse|UserPromptSubmit|Notification|Stop|PermissionRequest|Interrupt>")
		os.Exit(1)
	}
	event := args[0]

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}
	adapter := hookAdapterForEnvironment()
	payload, err := adapter.decode(raw)
	if err != nil {
		os.Exit(0)
	}

	payload.TabID = tabID
	payload.Raw = raw
	if (event == "SessionStart" || event == "SessionEnd" || event == "PreToolUse") && os.Getenv(hookProbeEnv) != "" {
		logHookProbe(event, os.Getenv("LGRASS_HOOK_VENDOR"), raw, payload)
	}

	proj, err := project.Resolve(payload.Cwd)
	if err != nil {
		if event == "PreToolUse" && payload.TabID != "" {
			if verdict := dangerDecision(payload); verdict != nil {
				adapter.emit(newHookResult("PreToolUse", "deny", []string{verdict.Message()}))
			}
		}
		os.Exit(0)
	}

	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		os.Exit(0)
	}
	defer store.Close()

	recordTabState(store, event, payload)

	var result hookResult
	switch event {
	case "SessionStart":
		result = hookSessionStart(store, payload, proj.Path)
	case "SessionEnd":
		hookSessionEnd(store, payload)
	case "PreToolUse":
		result = hookPreToolUse(store, payload, proj.Path)
	case "PostToolUse":
		result = hookPostToolUse(store, payload, proj.Path)
	case "UserPromptSubmit":
		result = hookUserPromptSubmit(store, payload)
	}
	adapter.emit(result)
}

// Temporary probe that writes raw SessionStart, SessionEnd and PreToolUse payloads to a /tmp file, for the Codex lifecycle investigation. Off unless LGRASS_HOOK_PROBE is set.
func logHookProbe(event, vendor string, raw []byte, payload hookEvent) {
	if vendor == "" {
		vendor = "claude"
	}
	f, err := os.OpenFile(hookProbeLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s event=%s vendor=%s source=%s session_id=%s tab_id=%t payload=%s\n", time.Now().UTC().Format(time.RFC3339Nano), event, vendor, payload.SessionSource, payload.SessionID, payload.TabID != "", raw)
}

func hookSessionStart(store *session.Store, payload hookEvent, projectPath string) hookResult {
	var startErr error
	if payload.PreserveSessionState {
		startErr = store.EnsureOpen(payload.SessionID)
	} else {
		startErr = store.Start(payload.SessionID)
	}
	if startErr != nil {
		fmt.Fprintf(os.Stderr, "lgrass: recording the session failed: %v\n", startErr)
	}
	if payload.TabID != "" && payload.SessionID != "" {
		store.RecordTabSession(payload.TabID, payload.SessionID)
	}
	if payload.TabID != "" {
		store.ClearMarks(payload.TabID)
	}

	var parts []string
	if lawsNeeded(payload) {
		if summary := lawsSummary(projectPath); summary != "" {
			parts = append(parts, summary)
		}
	}
	if payload.TabID != "" {
		parts = append(parts, session.FormatPrefixNote())
	}
	if member, group, ok := liveThinker(store, payload.TabID); ok {
		parts = append(parts, session.FormatThinkerStart(member, group))
	} else if member, group, ok := liveCollaborator(store, payload.TabID); ok {
		parts = append(parts, session.FormatCollaboratorStart(member, group))
	}
	parts = append(parts, notificationContext(store, payload.TabID)...)
	return newHookResult("SessionStart", "", parts)
}

func hookSessionEnd(store *session.Store, payload hookEvent) {
	store.End(payload.SessionID)
	if payload.TabID != "" && payload.SessionID != "" {
		store.RecordTabSession(payload.TabID, payload.SessionID)
	}
}

// A Claude tab spawned by lemongrass carries the laws in its system prompt, which a resumed session replays from its start.
func lawsNeeded(payload hookEvent) bool {
	return os.Getenv(lawsInPromptEnv) == "" || payload.SessionSource == "resume"
}

// Missing biblio/laws/summary.md is not an error -- most projects have no laws to deliver.
func lawsSummary(projectPath string) string {
	data, err := os.ReadFile(filepath.Join(projectPath, "biblio", "laws", "summary.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func hookPreToolUse(store *session.Store, payload hookEvent, projectPath string) hookResult {
	filePaths := toolFilePaths(payload)

	if payload.ToolName == "EnterPlanMode" && hasBiblio(projectPath) {
		return newHookResult("PreToolUse", "deny", []string{session.FormatPlanModeDeny()})
	}

	_, _, deny := thinkerGateDecision(store, payload)
	if deny != "" {
		return newHookResult("PreToolUse", "deny", []string{deny})
	}
	if deny := collaboratorGateDeny(store, payload); deny != "" {
		return newHookResult("PreToolUse", "deny", []string{deny})
	}

	if payload.TabID != "" {
		if verdict := dangerDecision(payload); verdict != nil {
			return newHookResult("PreToolUse", "deny", []string{verdict.Message()})
		}
	}

	if payload.EnforceBibliothek && hasBiblio(projectPath) {
		if deny := memoryFeedbackDeny(store, payload, projectPath); deny != "" {
			return newHookResult("PreToolUse", "deny", []string{deny})
		}
	}

	if hasBiblio(projectPath) {
		if deny := closeGateDeny(store, payload); deny != "" {
			return newHookResult("PreToolUse", "deny", []string{deny})
		}
		if deny := tempWriteDeny(payload, projectPath); deny != "" {
			return newHookResult("PreToolUse", "deny", []string{deny})
		}
	}

	var parts []string

	if isFileEdit(payload) {
		for _, filePath := range filePaths {
			if hits, err := store.RecentActivity(payload.SessionID, filePath, collisionWindow); err == nil && len(hits) > 0 {
				if w := session.FormatCollisionWarning(hits); w != "" {
					parts = append(parts, w)
				}
			}
		}
	}
	if os.Getenv(replayEnv) == "" {
		parts = append(parts, notificationContext(store, payload.TabID)...)
	}

	return newHookResult("PreToolUse", "allow", parts)
}

// The thinker this tab is in its live group, if any. Any lookup failure reads as not a thinker, so a store problem never blocks a tool call.
func liveThinker(store *session.Store, tab string) (session.Member, session.Group, bool) {
	if tab == "" {
		return session.Member{}, session.Group{}, false
	}
	group, member, err := store.LiveMembership(tab)
	if err != nil || member.Role != session.RoleThinker {
		return session.Member{}, session.Group{}, false
	}
	return member, group, true
}

// Applies the thinker gates and records a skill load. Returns the tab's member row, whether it is a thinker, and a deny message or "".
func thinkerGateDecision(store *session.Store, payload hookEvent) (session.Member, bool, string) {
	member, _, ok := liveThinker(store, payload.TabID)
	if !ok {
		return session.Member{}, false, ""
	}
	marks, err := store.Marks(payload.TabID)
	if err != nil {
		return member, true, ""
	}
	deny, loaded := thinkerGate(thinkerGateInput{
		ToolName:  payload.ToolName,
		ToolInput: payload.ToolInput,
		Required:  member.RequiredSkills(),
		Marks:     marks,
	})
	if loaded != "" {
		store.MarkReady(payload.TabID, session.SkillMark(loaded))
	}
	return member, true, deny
}

func hookPostToolUse(store *session.Store, payload hookEvent, projectPath string) hookResult {
	store.Touch(payload.SessionID)
	if payload.TabID != "" {
		store.TouchTab(payload.TabID)
	}

	if isFileEdit(payload) {
		for _, filePath := range toolFilePaths(payload) {
			store.LogFileActivity(payload.SessionID, filePath)
		}
	}

	var parts []string
	parts = append(parts, notificationContext(store, payload.TabID)...)

	if fire, err := store.IncrementNudgeCounter(payload.SessionID, nudgeThreshold); err == nil && fire {
		if liveness, err := store.Liveness(payload.SessionID, idleThreshold); err == nil {
			parts = append(parts, session.FormatNudge(liveness))
		}
		if hasBiblio(projectPath) {
			var userTips []string
			if tips, err := store.ListTips(); err == nil {
				for _, t := range tips {
					userTips = append(userTips, t.Message)
				}
			}
			parts = append(parts, session.RandomTipFrom(userTips))
		}
	}

	return newHookResult("PostToolUse", "", parts)
}

// The app holds typed nudges while the human writes a draft, so the human's own prompt carries whatever was held.
func hookUserPromptSubmit(store *session.Store, payload hookEvent) hookResult {
	return newHookResult("UserPromptSubmit", "", notificationContext(store, payload.TabID))
}

func hasBiblio(projectPath string) bool {
	info, err := os.Stat(filepath.Join(projectPath, "biblio"))
	return err == nil && info.IsDir()
}

// Surfaces the tab's pending notifications once, other kinds included since the tab is already working, then marks them sent so the next tool call does not repeat them.
func notificationContext(store *session.Store, tab string) []string {
	if tab == "" {
		return nil
	}
	pending, err := store.SurfaceableForTab(tab)
	if err != nil || pending.Empty() {
		return nil
	}
	store.MarkNotificationsSent(pending.RowIDs())
	return []string{store.NotificationText(pending)}
}

func hookAdapterForEnvironment() hookAdapter {
	if os.Getenv("LGRASS_HOOK_VENDOR") == "codex" {
		return codexHookAdapter{}
	}
	return claudeHookAdapter{}
}

type hookAdapter interface {
	decode([]byte) (hookEvent, error)
	emit(hookResult)
}

func isFileEdit(payload hookEvent) bool {
	return payload.ToolName == "Write" || payload.ToolName == "Edit" || payload.ToolName == "apply_patch"
}

func toolFilePaths(payload hookEvent) []string {
	var input fileToolInput
	if err := json.Unmarshal(payload.ToolInput, &input); err == nil && input.FilePath != "" {
		return []string{input.FilePath}
	}
	if payload.ToolName != "apply_patch" {
		return nil
	}
	var patch patchToolInput
	if err := json.Unmarshal(payload.ToolInput, &patch); err != nil {
		return nil
	}
	return patchFilePaths(patch.Command)
}

func patchFilePaths(command string) []string {
	prefixes := []string{"*** Add File: ", "*** Delete File: ", "*** Update File: "}
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(command, "\n") {
		for _, prefix := range prefixes {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			path := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
			break
		}
	}
	return paths
}
