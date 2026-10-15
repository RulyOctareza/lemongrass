package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
)

// replayEnv is set on the hook a replay runs, so a call it denies again is not held a second time.
const replayEnv = "LGRASS_HOOK_REPLAY"

const replayCheckTimeout = 10 * time.Second

// ReplayCall is what a held call runs: Command for Bash, FilePath and Content for Write.
type ReplayCall struct {
	Tool     string
	Cwd      string
	Command  string
	FilePath string
	Content  string
}

type writeToolInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func DecodeHeldCall(call session.HeldCall) (ReplayCall, error) {
	payload, err := claudeHookAdapter{}.decode(call.Payload)
	if err != nil {
		return ReplayCall{}, fmt.Errorf("hook: decoding held call %s: %w", call.ID, err)
	}
	out := ReplayCall{Tool: payload.ToolName, Cwd: payload.Cwd}
	switch payload.ToolName {
	case "Bash":
		var in shellToolInput
		if err := json.Unmarshal(payload.ToolInput, &in); err != nil || in.Command == "" {
			return ReplayCall{}, fmt.Errorf("hook: held call %s has no command", call.ID)
		}
		out.Command = in.Command
	case "Write":
		var in writeToolInput
		if err := json.Unmarshal(payload.ToolInput, &in); err != nil || in.FilePath == "" {
			return ReplayCall{}, fmt.Errorf("hook: held call %s has no file path", call.ID)
		}
		out.FilePath = in.FilePath
		if !filepath.IsAbs(out.FilePath) {
			out.FilePath = filepath.Join(payload.Cwd, out.FilePath)
		}
		out.Content = in.Content
	default:
		return ReplayCall{}, fmt.Errorf("hook: held call %s is a %s call, which replay does not run", call.ID, payload.ToolName)
	}
	return out, nil
}

// DescribeHeldCall shows a held call for review on sign, with heredoc bodies that only reach a file outside memory collapsed to a line count.
func DescribeHeldCall(call session.HeldCall, projectPath string) string {
	rc, err := DecodeHeldCall(call)
	if err != nil {
		return err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Held call %s, %s in %s:\n", call.ID, rc.Tool, rc.Cwd)
	if rc.Tool == "Write" {
		fmt.Fprintf(&b, "file %s\n%s", rc.FilePath, rc.Content)
	} else {
		b.WriteString(collapseNonMemoryHeredocs(rc.Command, rc.Cwd, projectPath))
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "If what it writes into memory is a pointer only, run `lgrass replay %s`. Otherwise do not replay it and send a corrected call.", call.ID)
	return b.String()
}

// collapseNonMemoryHeredocs collapses only data bodies that come before the command first names a memory directory, since a cd or variable after that point can carry a later write into memory.
func collapseNonMemoryHeredocs(command, cwd, projectPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return command
	}
	projectMemDir, _ := claudeMemoryDir(projectPath)
	if cwd != "" && isClaudeMemoryPath(filepath.Join(cwd, "x"), "", home, projectMemDir) {
		return command
	}
	spans, ok := shellHeredocs(command)
	if !ok {
		return command
	}
	masked := maskBodies(command, spans, func(sp heredocSpan) bool { return sp.dataOnly })
	first := firstMemoryMention(masked, projectMemDir)
	if first < 0 {
		return command
	}
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if !sp.dataOnly || sp.start > first || sp.start < pos {
			continue
		}
		b.WriteString(command[pos:sp.start])
		fmt.Fprintf(&b, "[%d lines not shown, written outside memory]\n", strings.Count(command[sp.start:sp.end], "\n"))
		pos = sp.end
	}
	b.WriteString(command[pos:])
	return b.String()
}

type claudeDecision struct {
	HookSpecificOutput struct {
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

// RecheckHeldCall runs the held payload through the PreToolUse hook of the binary that held it and returns its deny reason, or "" when it now passes; the vault serves its policy only to that binary, so an in-process check would drop the user policy.
func RecheckHeldCall(call session.HeldCall) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), replayCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, call.HookBinary, "hook", "PreToolUse")
	cmd.Stdin = bytes.NewReader(call.Payload)
	cmd.Env = append(withoutEnv(os.Environ(), "LGRASS_HOOK_VENDOR"), replayEnv+"=1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("hook: re-checking held call %s: %w", call.ID, err)
	}
	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		return "", nil
	}
	var d claudeDecision
	if err := json.Unmarshal(out, &d); err != nil {
		return "", errors.New("hook: re-checking held call " + call.ID + ": unreadable hook output")
	}
	if d.HookSpecificOutput.PermissionDecision == "deny" {
		return d.HookSpecificOutput.PermissionDecisionReason, nil
	}
	return "", nil
}

func withoutEnv(env []string, name string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}
