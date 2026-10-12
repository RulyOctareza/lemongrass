package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	gateTabLeader  = "aaaaaaaa-1111-4111-8111-111111111111"
	gateTabClaude  = "bbbbbbbb-2222-4222-8222-222222222222"
	gateTabCodex   = "cccccccc-3333-4333-8333-333333333333"
	gateTabOutside = "dddddddd-4444-4444-8444-444444444444"
)

func storeWithGroup(t *testing.T, claudeSkills []string) (*session.Store, session.Group) {
	t.Helper()
	store := openHookTestStore(t)
	group, err := store.CreateGroup("review", session.Member{TabID: gateTabLeader, Label: "lead", Vendor: "claude"}, []session.Member{
		{TabID: gateTabClaude, Label: "reviewer", Vendor: "claude", Skills: claudeSkills},
		{TabID: gateTabCodex, Label: "tester", Vendor: "codex"},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return store, group
}

func preTool(tab, tool string, input any) hookEvent {
	raw, _ := json.Marshal(input)
	return hookEvent{SessionID: "s-" + strings.TrimSpace(tab), ToolName: tool, ToolInput: raw, TabID: tab, EnforceBibliothek: true}
}

func projectWithBiblio(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "biblio"), 0o755)
	return dir
}

func TestOnlyThinkersAreGated(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := t.TempDir()

	for _, tab := range []string{gateTabLeader, gateTabOutside, ""} {
		result := hookPreToolUse(store, preTool(tab, "Bash", map[string]string{"command": "ls"}), dir)
		if result.PermissionDecision == "deny" {
			t.Errorf("tab %q was gated: %s", tab, result.PermissionDecisionReason)
		}
	}
	result := hookPreToolUse(store, preTool(gateTabClaude, "Bash", map[string]string{"command": "ls"}), dir)
	if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "lgrass-howtobe-thinker") {
		t.Errorf("thinker was not gated: %+v", result)
	}
}

func TestLoadingTheSkillLiftsGateOneAndSurvivesToLaterCalls(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := t.TempDir()

	load := hookPreToolUse(store, preTool(gateTabClaude, "Skill", map[string]string{"skill": "lgrass-howtobe-thinker"}), dir)
	if load.PermissionDecision == "deny" {
		t.Fatalf("the skill load was denied: %s", load.PermissionDecisionReason)
	}
	for _, command := range []string{"ls", "go test ./..."} {
		result := hookPreToolUse(store, preTool(gateTabClaude, "Bash", map[string]string{"command": command}), dir)
		if result.PermissionDecision == "deny" {
			t.Errorf("%q denied after the skill was loaded: %s", command, result.PermissionDecisionReason)
		}
	}
}

func TestAThinkerWithARequiredSkillListMustLoadThemAll(t *testing.T) {
	store, _ := storeWithGroup(t, []string{"lgrass-connector"})
	dir := t.TempDir()

	hookPreToolUse(store, preTool(gateTabClaude, "Skill", map[string]string{"skill": "lgrass-howtobe-thinker"}), dir)
	result := hookPreToolUse(store, preTool(gateTabClaude, "Bash", map[string]string{"command": "ls"}), dir)
	if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "lgrass-connector") {
		t.Fatalf("the second required skill was not enforced: %+v", result)
	}
	hookPreToolUse(store, preTool(gateTabClaude, "Skill", map[string]string{"skill": "lgrass-connector"}), dir)
	if result := hookPreToolUse(store, preTool(gateTabClaude, "Bash", map[string]string{"command": "ls"}), dir); result.PermissionDecision == "deny" {
		t.Errorf("still denied after both skills: %s", result.PermissionDecisionReason)
	}
}

func TestCodexThinkerNeedsSkillsButNoListener(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := t.TempDir()
	ls := map[string]any{"command": []string{"bash", "-lc", "ls"}}
	result := hookPreToolUse(store, preTool(gateTabCodex, "shell", ls), dir)
	if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "lgrass-howtobe-thinker") {
		t.Fatalf("required skill did not hold the gate: %+v", result)
	}
	read := map[string]any{"command": []string{"bash", "-lc", "cat ~/.codex/skills/lgrass-howtobe-thinker/SKILL.md"}}

	if load := hookPreToolUse(store, preTool(gateTabCodex, "shell", read), dir); load.PermissionDecision == "deny" {
		t.Fatalf("the skill read was denied: %s", load.PermissionDecisionReason)
	}
	if result := hookPreToolUse(store, preTool(gateTabCodex, "shell", ls), dir); result.PermissionDecision == "deny" {
		t.Errorf("denied after the skill loaded without a listener: %s", result.PermissionDecisionReason)
	}
}

func TestDisbandingFreesTheThinkers(t *testing.T) {
	store, group := storeWithGroup(t, nil)
	store.DisbandGroup(group.ID)

	result := hookPreToolUse(store, preTool(gateTabCodex, "Bash", map[string]string{"command": "ls"}), t.TempDir())
	if result.PermissionDecision == "deny" {
		t.Errorf("a disbanded group's thinker is still gated: %s", result.PermissionDecisionReason)
	}
}

func TestBibliothekIsNeverRequiredOfAClaudeThinker(t *testing.T) {
	store, _ := storeWithGroup(t, []string{"bibliothek"})
	dir := projectWithBiblio(t)
	store.MarkReady(gateTabClaude, session.SkillMark("lgrass-howtobe-thinker"))
	if result := hookPreToolUse(store, preTool(gateTabClaude, "Bash", map[string]string{"command": "ls"}), dir); result.PermissionDecision == "deny" {
		t.Errorf("a Claude thinker was held to load bibliothek: %s", result.PermissionDecisionReason)
	}
}

func TestPlainTabNeedsNoBibliothekSkill(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := projectWithBiblio(t)
	if result := hookPreToolUse(store, preTool(gateTabOutside, "Bash", map[string]string{"command": "ls"}), dir); result.PermissionDecision == "deny" {
		t.Errorf("a plain tab was denied: %s", result.PermissionDecisionReason)
	}
}

func TestFoundationalRulesStillApplyToThinkers(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := projectWithBiblio(t)
	store.MarkReady(gateTabClaude, session.SkillMark("lgrass-howtobe-thinker"))

	result := hookPreToolUse(store, preTool(gateTabClaude, "EnterPlanMode", map[string]string{}), dir)
	if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "EnterPlanMode") {
		t.Errorf("a thinker was allowed into plan mode: %+v", result)
	}
}

func TestSessionStartSendsLawsAndTheStartLineAndResetsMarks(t *testing.T) {
	t.Setenv(lawsInPromptEnv, "")
	store, _ := storeWithGroup(t, []string{"lgrass-connector"})
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "biblio", "laws"), 0o755)
	os.WriteFile(filepath.Join(dir, "biblio", "laws", "summary.md"), []byte("# Laws summary\n- a law"), 0o600)
	store.MarkReady(gateTabClaude, session.SkillMark("lgrass-howtobe-thinker"))

	result := hookSessionStart(store, hookEvent{SessionID: "s1", TabID: gateTabClaude}, dir)
	for _, want := range []string{"a law", `thinker "reviewer"`, "lgrass-howtobe-thinker, lgrass-connector", "lgrass workgroup thread"} {
		if !strings.Contains(result.AdditionalContext, want) {
			t.Errorf("start context missing %q:\n%s", want, result.AdditionalContext)
		}
	}
	if strings.Contains(result.AdditionalContext, "lgrass listen") {
		t.Error("a Claude thinker was told to run a listener")
	}
	if marks, _ := store.Marks(gateTabClaude); len(marks) != 0 {
		t.Errorf("marks survived SessionStart: %v", marks)
	}

	codex := hookSessionStart(store, hookEvent{SessionID: "s2", TabID: gateTabCodex}, dir)
	if strings.Contains(codex.AdditionalContext, "lgrass listen") {
		t.Error("a Codex thinker was told to keep a listener running")
	}
	for _, want := range []string{`thinker "tester"`, "lgrass-howtobe-thinker", "lgrass workgroup thread"} {
		if !strings.Contains(codex.AdditionalContext, want) {
			t.Errorf("Codex start context missing %q", want)
		}
	}
	leader := hookSessionStart(store, hookEvent{SessionID: "s3", TabID: gateTabLeader}, dir)
	if strings.Contains(leader.AdditionalContext, "thinker") || !strings.Contains(leader.AdditionalContext, "a law") {
		t.Errorf("the leader's start context = %q, want laws only", leader.AdditionalContext)
	}
}
