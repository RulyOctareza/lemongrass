// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package gatekeeper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
)

func registerTestProject(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	p, err := project.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestIPCPolicyRoundTrip(t *testing.T) {
	client := startTestServer(t)
	if err := client.SetPassphrase(testRootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if current, err := client.CurrentPolicy(""); err != nil || len(current.Binaries) != 0 {
		t.Fatalf("CurrentPolicy before any policy = %+v, %v", current, err)
	}

	policy := guard.Policy{Binaries: []string{"make"}, Paths: []string{"~/private"}}
	if err := client.PutPolicy(testRootSecret, "", policy); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	current, err := client.CurrentPolicy("")
	if err != nil {
		t.Fatalf("CurrentPolicy: %v", err)
	}
	if current.Binaries[0] != "make" || current.Paths[0] != "~/private" {
		t.Errorf("CurrentPolicy = %+v", current)
	}
	global, projects, err := client.Policies()
	if err != nil || global.Binaries[0] != "make" || len(projects) != 0 {
		t.Errorf("Policies = %+v, %+v, %v", global, projects, err)
	}
	if err := client.MigratePolicy(testRootSecret); err != nil {
		t.Errorf("MigratePolicy with nothing sealed: %v", err)
	}
	rules, err := client.PolicyCatalog()
	if err != nil || len(rules) == 0 {
		t.Errorf("PolicyCatalog = %d rules, %v", len(rules), err)
	}
}

func TestIPCProjectPolicyLayersOverGlobal(t *testing.T) {
	projectID := registerTestProject(t)
	client := startTestServer(t)
	if err := client.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	if err := client.PutPolicy(testRootSecret, "", guard.Policy{Binaries: []string{"make", "cowsay"}}); err != nil {
		t.Fatal(err)
	}
	project := guard.Policy{Rules: map[string]string{"git-add": "allow"}, Binaries: []string{"cargo"}, ExcludeBinaries: []string{"cowsay"}}
	if err := client.PutPolicy(testRootSecret, projectID, project); err != nil {
		t.Fatalf("PutPolicy for a project: %v", err)
	}
	current, err := client.CurrentPolicy(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(current.Binaries, ",") != "make,cargo" || current.Rules["git-add"] != "allow" {
		t.Errorf("project policy = %+v", current)
	}
	if other, _ := client.CurrentPolicy("other"); strings.Join(other.Binaries, ",") != "make,cowsay" || len(other.Rules) != 0 {
		t.Errorf("project with nothing stored = %+v, want global over the defaults", other)
	}
	_, projects, err := client.Policies()
	if err != nil || projects[projectID].ExcludeBinaries[0] != "cowsay" {
		t.Errorf("Policies projects = %+v, %v", projects, err)
	}
	err = client.PutPolicy(testRootSecret, "not-registered", guard.Policy{})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("policy for an unregistered project = %v", err)
	}
	err = client.PutPolicy(testRootSecret, "", guard.Policy{Rules: map[string]string{"git-add": "allow"}})
	if err == nil || !strings.Contains(err.Error(), "only blocked binaries and paths") {
		t.Errorf("rule modes in the global policy = %v", err)
	}
}

func TestIPCPolicyRejectsWhatTheGuardForbids(t *testing.T) {
	client := startTestServer(t)
	if err := client.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	err := client.PutPolicy(testRootSecret, "", guard.Policy{Binaries: []string{"lgrass"}})
	if err == nil || !strings.Contains(err.Error(), "lgrass cannot be blocked") {
		t.Errorf("blocking lgrass = %v", err)
	}
	projectID := registerTestProject(t)
	err = client.PutPolicy(testRootSecret, projectID, guard.Policy{Rules: map[string]string{"secret-path": "allow"}})
	if err == nil || !strings.Contains(err.Error(), "cannot be changed") {
		t.Errorf("changing a locked rule = %v", err)
	}
	if current, _ := client.CurrentPolicy(""); len(current.Binaries)+len(current.Rules) != 0 {
		t.Errorf("a rejected policy was stored: %+v", current)
	}
}

func TestIPCPolicyWrongPassphrase(t *testing.T) {
	client := startTestServer(t)
	if err := client.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	if err := client.PutPolicy("not the passphrase", "", guard.Policy{}); err == nil {
		t.Error("PutPolicy accepted a wrong passphrase")
	}
	if err := client.MigratePolicy("not the passphrase"); err == nil {
		t.Error("MigratePolicy accepted a wrong passphrase")
	}
}

func writeRegistry(t *testing.T, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".lemongrass", "projects.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectPoliciesFollowTheRegistry(t *testing.T) {
	kept := registerTestProject(t)
	gone, err := project.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	b, err := NewBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetPassphrase(testRootSecret); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{kept, gone.ID} {
		if err := b.PutPolicy(testRootSecret, id, []byte(`{"binaries":["make"]}`)); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := project.List()
	if err != nil {
		t.Fatal(err)
	}
	keptOnly, err := json.Marshal(registry[:1])
	if err != nil {
		t.Fatal(err)
	}

	writeRegistry(t, "not json")
	if err := b.PutPolicy(testRootSecret, "", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, projects := b.Policies(); len(projects) != 2 {
		t.Errorf("an unreadable registry pruned: %d projects left", len(projects))
	}

	writeRegistry(t, string(keptOnly))
	reopened, err := NewBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, projects := reopened.Policies(); len(projects) != 1 || projects[kept] == nil {
		t.Errorf("projects after a restart = %q, want only %s", projects, kept)
	}

	writeRegistry(t, "[]")
	if err := reopened.PutPolicy(testRootSecret, "", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, projects := reopened.Policies(); len(projects) != 1 {
		t.Errorf("an empty registry pruned: %d projects left", len(projects))
	}
}
