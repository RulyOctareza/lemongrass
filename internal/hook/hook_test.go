package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const hookTestProjectID = "hooktestproj"

func openHookTestStore(t *testing.T) *session.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := session.Open(session.DBPath(), hookTestProjectID)
	if err != nil {
		t.Fatalf("session.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestPatchFilePaths(t *testing.T) {
	command := "*** Begin Patch\n*** Update File: one.txt\n*** Add File: two.txt\n*** Update File: one.txt\n*** Delete File: three.txt\n*** End Patch"
	got := patchFilePaths(command)
	want := []string{"one.txt", "two.txt", "three.txt"}
	if len(got) != len(want) {
		t.Fatalf("patchFilePaths() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("patchFilePaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodexHookAdapterPreservesSessionState(t *testing.T) {
	adapter := codexHookAdapter{}
	event, err := adapter.decode([]byte(`{"session_id":"session-a","cwd":"/workspace","source":"compact"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !event.PreserveSessionState {
		t.Error("Codex event should preserve existing session state")
	}
	if event.SessionSource != "compact" {
		t.Errorf("SessionSource = %q, want compact", event.SessionSource)
	}
}

func TestEnterPlanModeDeniedInBibliothekProject(t *testing.T) {
	store := openHookTestStore(t)
	projectPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := hookEvent{SessionID: "session-a", ToolName: "EnterPlanMode"}
	result := hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision != "deny" {
		t.Errorf("PermissionDecision = %q, want deny", result.PermissionDecision)
	}
	if !strings.Contains(result.PermissionDecisionReason, "EnterPlanMode") {
		t.Errorf("PermissionDecisionReason = %q, want it to mention EnterPlanMode", result.PermissionDecisionReason)
	}
}

func TestEnterPlanModeAllowedWithoutBibliothek(t *testing.T) {
	store := openHookTestStore(t)
	projectPath := t.TempDir()

	payload := hookEvent{SessionID: "session-a", ToolName: "EnterPlanMode"}
	result := hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision == "deny" {
		t.Errorf("PermissionDecision = %q, want no deny for a project without biblio/", result.PermissionDecision)
	}
}

func TestClaudeMemoryDirReplacesSlashesWithDashes(t *testing.T) {
	t.Setenv("HOME", "/home/faizal")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	got, err := claudeMemoryDir("/mnt/data/Projects/lemongrass")
	if err != nil {
		t.Fatalf("claudeMemoryDir: %v", err)
	}
	want := "/home/faizal/.claude/projects/-mnt-data-Projects-lemongrass/memory/"
	if got != want {
		t.Errorf("claudeMemoryDir() = %q, want %q", got, want)
	}
}

func TestClaudeMemoryDirFollowsTheConfigDirOfAnAccount(t *testing.T) {
	t.Setenv("HOME", "/home/faizal")
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/faizal/.claude-work")
	got, err := claudeMemoryDir("/mnt/data/Projects/lemongrass")
	if err != nil {
		t.Fatalf("claudeMemoryDir: %v", err)
	}
	want := "/home/faizal/.claude-work/projects/-mnt-data-Projects-lemongrass/memory/"
	if got != want {
		t.Errorf("claudeMemoryDir() = %q, want %q", got, want)
	}
}

func TestMemoryFeedbackDenyUntilSigned(t *testing.T) {
	store := openHookTestStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}

	memDir, err := claudeMemoryDir(projectPath)
	if err != nil {
		t.Fatalf("claudeMemoryDir: %v", err)
	}
	memFile := filepath.Join(memDir, "feedback_testing.md")

	payload := hookEvent{
		SessionID:         "session-a",
		ToolName:          "Write",
		ToolInput:         json.RawMessage(`{"file_path":"` + memFile + `"}`),
		EnforceBibliothek: true,
	}
	result := hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision != "deny" {
		t.Fatalf("PermissionDecision = %q, want deny before signing", result.PermissionDecision)
	}
	if !strings.Contains(result.PermissionDecisionReason, "i-write-memory") {
		t.Errorf("PermissionDecisionReason = %q, want it to mention i-write-memory", result.PermissionDecisionReason)
	}

	if err := store.Sign("session-a", memoryWriteWord); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	result = hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision == "deny" {
		t.Errorf("PermissionDecision = %q, want no deny after signing", result.PermissionDecision)
	}
}

func TestMemoryFeedbackDenyIgnoresNonMemoryPaths(t *testing.T) {
	store := openHookTestStore(t)
	t.Setenv("HOME", t.TempDir())
	projectPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := hookEvent{
		SessionID:         "session-a",
		ToolName:          "Write",
		ToolInput:         json.RawMessage(`{"file_path":"` + filepath.Join(projectPath, "main.go") + `"}`),
		EnforceBibliothek: true,
	}
	result := hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision == "deny" {
		t.Errorf("PermissionDecision = %q, want no deny for a file outside the memory directory", result.PermissionDecision)
	}
}

func TestMemoryFeedbackDenyRequiresBiblio(t *testing.T) {
	store := openHookTestStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectPath := t.TempDir()

	memDir, err := claudeMemoryDir(projectPath)
	if err != nil {
		t.Fatalf("claudeMemoryDir: %v", err)
	}
	memFile := filepath.Join(memDir, "feedback_testing.md")

	payload := hookEvent{
		SessionID:         "session-a",
		ToolName:          "Write",
		ToolInput:         json.RawMessage(`{"file_path":"` + memFile + `"}`),
		EnforceBibliothek: true,
	}
	result := hookPreToolUse(store, payload, projectPath)
	if result.PermissionDecision == "deny" {
		t.Errorf("PermissionDecision = %q, want no deny for a project without biblio/", result.PermissionDecision)
	}
}

func TestSessionEndRecordsTheLastTabSession(t *testing.T) {
	store := openHookTestStore(t)

	hookSessionEnd(store, hookEvent{SessionID: "session-a", TabID: "tab-1"})
	hookSessionEnd(store, hookEvent{SessionID: "session-b", TabID: "tab-1"})

	tabs, err := store.TabSessions()
	if err != nil {
		t.Fatalf("TabSessions: %v", err)
	}
	if len(tabs) != 1 || tabs["tab-1"] != "session-b" {
		t.Errorf("TabSessions = %v, want tab-1=session-b", tabs)
	}
}

func TestSessionStartWithoutTabIDRecordsNoTabSession(t *testing.T) {
	store := openHookTestStore(t)

	hookSessionStart(store, hookEvent{SessionID: "session-a"}, t.TempDir())

	tabs, err := store.TabSessions()
	if err != nil {
		t.Fatalf("TabSessions: %v", err)
	}
	if len(tabs) != 0 {
		t.Errorf("TabSessions = %v, want none", tabs)
	}
}

func projectWithLaws(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	laws := filepath.Join(dir, "biblio", "laws")
	if err := os.MkdirAll(laws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(laws, "summary.md"), []byte("# Laws summary\n\n- a rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSessionStartDeliversLaws(t *testing.T) {
	t.Setenv(lawsInPromptEnv, "")
	result := hookSessionStart(openHookTestStore(t), hookEvent{SessionID: "s", SessionSource: "startup"}, projectWithLaws(t))
	if !strings.Contains(result.AdditionalContext, "# Laws summary") {
		t.Errorf("context = %q, want the laws summary", result.AdditionalContext)
	}
}

func TestSessionStartSkipsLawsAlreadyInThePrompt(t *testing.T) {
	t.Setenv(lawsInPromptEnv, "1")
	project := projectWithLaws(t)
	for source, want := range map[string]bool{"startup": false, "clear": false, "compact": false, "resume": true} {
		result := hookSessionStart(openHookTestStore(t), hookEvent{SessionID: "s", SessionSource: source}, project)
		if got := strings.Contains(result.AdditionalContext, "# Laws summary"); got != want {
			t.Errorf("source %s: laws delivered = %v, want %v", source, got, want)
		}
	}
}
