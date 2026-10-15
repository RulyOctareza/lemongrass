package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/session"
)

const memoryWriteWord = session.MemoryWriteWord

// Short enough to resurface again within the same session if memory writes are still happening.
const memoryFeedbackSignatureTTL = 30 * time.Minute

type notebookToolInput struct {
	NotebookPath string `json:"notebook_path"`
}

type shellToolInput struct {
	Command string `json:"command"`
}

var (
	claudeConfigDirName     = regexp.MustCompile(`^\.claude(-[a-z0-9-]+)?$`)
	claudeProjectsInCommand = regexp.MustCompile(`\.claude(-[a-z0-9-]+)?/projects`)
	shellDevNullRedirect    = regexp.MustCompile(`\d*>&?\s*/dev/null`)
	shellFdDuplication      = regexp.MustCompile(`\d*>&\d+`)
	shellSegmentSeparator   = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	findWriteAction         = regexp.MustCompile(`\s-(delete|exec|execdir|ok|okdir|fprint|fprint0|fprintf|fls)\b`)
	shellWriteIndicator     = regexp.MustCompile(`>|<<|\btee\b|\bsed\b[^|;&]*\s-[a-zA-Z]*i|--in-place|\b(cp|mv|install|dd|truncate|rm|ln|touch|patch|rsync|ed|ex|vi|vim|nano|emacs|curl|wget|python[0-9.]*|node|ruby|perl|php|deno|bun)\b|\b(sh|bash|zsh)\s+-c\b|\beval\b`)
)

// memoryFeedbackDeny gates every route that can write into a Claude Code memory directory (file tools, patches, notebooks, shell commands) behind a sign, once per session per memoryFeedbackSignatureTTL.
func memoryFeedbackDeny(store *session.Store, payload hookEvent, projectPath string) string {
	if !touchesClaudeMemory(payload, projectPath) {
		return ""
	}
	signedAt, err := store.SignedAt(payload.SessionID, memoryWriteWord)
	if err == nil && !signedAt.IsZero() && time.Since(signedAt) <= memoryFeedbackSignatureTTL {
		return ""
	}
	if id := holdMemoryCall(store, payload); id != "" {
		return session.FormatMemoryHeldDeny(id)
	}
	return session.FormatMemoryFeedbackDeny()
}

// holdMemoryCall returns "" when the call is not one replay can run, is itself a replay, or cannot be stored, which leaves the plain deny.
func holdMemoryCall(store *session.Store, payload hookEvent) string {
	if payload.ToolName != "Bash" && payload.ToolName != "Write" {
		return ""
	}
	if payload.SessionID == "" || len(payload.Raw) == 0 || os.Getenv(replayEnv) != "" {
		return ""
	}
	binary, err := os.Executable()
	if err != nil {
		return ""
	}
	id, err := store.HoldCall(session.HeldCall{
		SessionID:  payload.SessionID,
		TabID:      payload.TabID,
		ToolName:   payload.ToolName,
		Payload:    payload.Raw,
		HookBinary: binary,
	})
	if err != nil {
		return ""
	}
	return id
}

func touchesClaudeMemory(payload hookEvent, projectPath string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	projectMemDir, _ := claudeMemoryDir(projectPath)

	if payload.ToolName == "Bash" {
		return shellTouchesClaudeMemory(payload, home, projectMemDir)
	}
	if !isMemoryWriteTool(payload.ToolName) {
		return false
	}
	for _, p := range memoryToolPaths(payload) {
		if isClaudeMemoryPath(p, payload.Cwd, home, projectMemDir) {
			return true
		}
	}
	return false
}

func isMemoryWriteTool(name string) bool {
	switch name {
	case "Write", "Edit", "MultiEdit", "NotebookEdit", "apply_patch":
		return true
	}
	return false
}

func memoryToolPaths(payload hookEvent) []string {
	paths := toolFilePaths(payload)
	var nb notebookToolInput
	if err := json.Unmarshal(payload.ToolInput, &nb); err == nil && nb.NotebookPath != "" {
		paths = append(paths, nb.NotebookPath)
	}
	return paths
}

// isClaudeMemoryPath matches any project's memory directory under the projects directory of ~/.claude or of any ~/.claude-<account>, not only this project's, so a slug-algorithm mismatch or a sibling project's directory does not open a gap.
func isClaudeMemoryPath(p, cwd, home, projectMemDir string) bool {
	p = guard.ResolvePath(p, cwd, home)
	if projectMemDir != "" && strings.HasPrefix(p+string(filepath.Separator), projectMemDir) {
		return true
	}
	prefix := home + string(filepath.Separator)
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(p, prefix), string(filepath.Separator))
	return len(parts) >= 4 && claudeConfigDirName.MatchString(parts[0]) && parts[1] == "projects" && parts[3] == "memory"
}

// A shell command counts when it names a memory directory, or runs from inside one, and contains something that can write. A command whose every segment naming a memory directory is a plain read (cat, ls, grep, head) passes, whatever its other segments write. A heredoc body that only reaches a file is data, so only its command line is judged.
func shellTouchesClaudeMemory(payload hookEvent, home, projectMemDir string) bool {
	var in shellToolInput
	if err := json.Unmarshal(payload.ToolInput, &in); err != nil || in.Command == "" {
		return false
	}
	command := in.Command
	if spans, ok := shellHeredocs(command); ok {
		command = maskBodies(command, spans, func(sp heredocSpan) bool { return sp.dataOnly })
	}
	stripped := shellDevNullRedirect.ReplaceAllString(command, "")
	if !shellWriteIndicator.MatchString(stripped) && !findWriteAction.MatchString(stripped) {
		return false
	}
	if payload.Cwd != "" && isClaudeMemoryPath(filepath.Join(payload.Cwd, "x"), "", home, projectMemDir) {
		return true
	}
	cmd := strings.ReplaceAll(command, "\\", "")
	if !namesClaudeMemory(cmd, projectMemDir) {
		return false
	}
	return !memoryOnlyRead(cmd, projectMemDir)
}

func namesClaudeMemory(cmd, projectMemDir string) bool {
	return claudeProjectsInCommand.MatchString(cmd) && strings.Contains(cmd, "memory") ||
		projectMemDir != "" && strings.Contains(cmd, strings.TrimSuffix(projectMemDir, string(filepath.Separator)))
}

// memoryOnlyRead reports whether every segment that names a memory directory is a plain read with no write in it, so a cd, loop or variable assignment that carries the path into another segment never passes.
func memoryOnlyRead(cmd, projectMemDir string) bool {
	cmd = shellFdDuplication.ReplaceAllString(shellDevNullRedirect.ReplaceAllString(cmd, ""), "")
	for _, seg := range shellSegmentSeparator.Split(cmd, -1) {
		if namesClaudeMemory(seg, projectMemDir) && !isPlainReadSegment(seg) {
			return false
		}
	}
	return true
}

func isPlainReadSegment(seg string) bool {
	if shellWriteIndicator.MatchString(seg) {
		return false
	}
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "cat", "ls", "grep", "rg", "head", "tail", "wc", "stat", "file":
		return true
	case "find":
		return !findWriteAction.MatchString(seg)
	}
	return false
}

// claudeMemoryDir mirrors Claude Code's own project-slug algorithm: the resolved absolute project path with every path separator replaced by "-". projectPath must already be symlink-resolved, which project.Resolve guarantees.
func claudeMemoryDir(projectPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		configDir = filepath.Join(home, ".claude")
	}
	slug := strings.ReplaceAll(projectPath, string(filepath.Separator), "-")
	return filepath.Join(configDir, "projects", slug, "memory") + string(filepath.Separator), nil
}
