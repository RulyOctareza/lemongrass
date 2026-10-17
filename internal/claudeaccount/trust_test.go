package claudeaccount

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeClaudeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readClaudeJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}

func trusted(root map[string]any, project string) any {
	projects, _ := root["projects"].(map[string]any)
	entry, _ := projects[project].(map[string]any)
	return entry["hasTrustDialogAccepted"]
}

func TestTrustAddsEntryAndKeepsTheRest(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	path := a.ClaudeJSONPath("")
	writeClaudeJSON(t, path, `{"userID":"u1","projects":{"/p/other":{"hasTrustDialogAccepted":true},"/p/app":{"allowedTools":["x"],"hasTrustDialogAccepted":false}}}`)

	if err := a.Trust("", "/p/app"); err != nil {
		t.Fatal(err)
	}
	root := readClaudeJSON(t, path)
	if trusted(root, "/p/app") != true {
		t.Errorf("trust not written: %v", root)
	}
	if root["userID"] != "u1" || trusted(root, "/p/other") != true {
		t.Errorf("other keys changed: %v", root)
	}
	entry := root["projects"].(map[string]any)["/p/app"].(map[string]any)
	if tools, _ := entry["allowedTools"].([]any); len(tools) != 1 {
		t.Errorf("the project's other keys changed: %v", entry)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestTrustCreatesProjectsWhenAbsent(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	path := a.ClaudeJSONPath("work")
	writeClaudeJSON(t, path, `{"userID":"u2"}`)
	if err := a.Trust("work", "/p/app"); err != nil {
		t.Fatal(err)
	}
	if trusted(readClaudeJSON(t, path), "/p/app") != true {
		t.Error("trust not written into the account file")
	}
}

func TestTrustLeavesAnAcceptedEntryUntouched(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	path := a.ClaudeJSONPath("")
	original := `{"projects":{"/p/app":{"hasTrustDialogAccepted":true}}}`
	writeClaudeJSON(t, path, original)
	if err := a.Trust("", "/p/app"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Errorf("an accepted entry was rewritten: %s", data)
	}
}

func TestTrustSkipsAMissingFile(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	if err := a.Trust("work", "/p/app"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.ClaudeJSONPath("work")); !os.IsNotExist(err) {
		t.Errorf("a missing file must stay missing: %v", err)
	}
}

func TestTrustRefusesAFileThatIsNotAnObject(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	writeClaudeJSON(t, a.ClaudeJSONPath(""), `[1]`)
	if err := a.Trust("", "/p/app"); err == nil {
		t.Error("a non-object file must be refused")
	}
}

func TestTrustAllCoversPrimaryAndAccounts(t *testing.T) {
	a := Accounts{Home: t.TempDir()}
	if err := a.saveNames([]string{"work", "side"}); err != nil {
		t.Fatal(err)
	}
	writeClaudeJSON(t, a.ClaudeJSONPath(""), `{}`)
	writeClaudeJSON(t, a.ClaudeJSONPath("work"), `{}`)
	if err := a.TrustAll("/p/app"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "work"} {
		if trusted(readClaudeJSON(t, a.ClaudeJSONPath(name)), "/p/app") != true {
			t.Errorf("account %q not trusted", name)
		}
	}
	if _, err := os.Stat(a.ClaudeJSONPath("side")); !os.IsNotExist(err) {
		t.Errorf("an account that never logged in must stay without a file: %v", err)
	}
}
