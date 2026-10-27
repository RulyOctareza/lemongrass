// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"errors"
	"strings"
	"testing"
)

func TestPolicyValidate(t *testing.T) {
	cases := []struct {
		name   string
		policy Policy
		ok     bool
	}{
		{"empty", Policy{}, true},
		{"allow a catalog rule", Policy{Rules: map[string]string{"rm-plain": "allow"}}, true},
		{"deny an approval rule", Policy{Rules: map[string]string{"git-add": "deny"}}, true},
		{"unknown rule", Policy{Rules: map[string]string{"nope": "allow"}}, false},
		{"locked catalog rule", Policy{Rules: map[string]string{"keyring": "allow"}}, false},
		{"locked engine rule", Policy{Rules: map[string]string{"secret-path": "allow"}}, false},
		{"bad mode", Policy{Rules: map[string]string{"rm-plain": "maybe"}}, false},
		{"binary", Policy{Binaries: []string{"make"}}, true},
		{"lgrass binary", Policy{Binaries: []string{"lgrass"}}, false},
		{"lgrass binary uppercase", Policy{Binaries: []string{"LGRASS"}}, false},
		{"binary with slash", Policy{Binaries: []string{"/usr/bin/make"}}, false},
		{"binary with space", Policy{Binaries: []string{"make all"}}, false},
		{"empty binary", Policy{Binaries: []string{""}}, false},
		{"home path", Policy{Paths: []string{"~/private"}}, true},
		{"absolute glob", Policy{Paths: []string{"/data/*/secret"}}, true},
		{"relative path", Policy{Paths: []string{"private"}}, false},
		{"broken glob", Policy{Paths: []string{"/data/["}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.Validate()
			if tc.ok && err != nil {
				t.Errorf("Validate = %v, want nil", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidPolicy) {
				t.Errorf("Validate = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestParsePolicy(t *testing.T) {
	policy, err := ParsePolicy(nil)
	if err != nil || len(policy.Rules)+len(policy.Binaries)+len(policy.Paths) != 0 {
		t.Errorf("empty data = %+v, %v", policy, err)
	}
	if _, err := ParsePolicy([]byte("{")); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("bad json error = %v", err)
	}
	policy, err = ParsePolicy([]byte(`{"rules":{"rm-plain":"allow"},"binaries":["make"],"paths":["~/x"]}`))
	if err != nil || policy.Rules["rm-plain"] != "allow" || policy.Binaries[0] != "make" || policy.Paths[0] != "~/x" {
		t.Errorf("parsed = %+v, %v", policy, err)
	}
}

func TestPolicyOverrides(t *testing.T) {
	decide := func(command string, policy Policy) string {
		verdict := Decide(bashInput(t, command), policy)
		if verdict == nil {
			return ""
		}
		return verdict.RuleID
	}
	if got := decide("rm notes.txt", Policy{}); got != "rm-plain" {
		t.Fatalf("baseline = %q", got)
	}
	if got := decide("rm notes.txt", Policy{Rules: map[string]string{"rm-plain": "allow"}}); got != "" {
		t.Errorf("allowed rule still fired: %q", got)
	}
	if got := decide("rm -rf x", Policy{Rules: map[string]string{"rm-force": "allow"}}); got != "rm-plain" {
		t.Errorf("rm -rf with only rm-force allowed = %q, want the rm-plain approval", got)
	}
	both := Policy{Rules: map[string]string{"rm-force": "allow", "rm-plain": "allow"}}
	if got := decide("rm -rf x", both); got != "" {
		t.Errorf("rm -rf with both allowed = %q", got)
	}
	if verdict := Decide(bashInput(t, "rm -rf x"), Policy{Rules: map[string]string{"rm-force": "approval", "rm-plain": "approval"}}); verdict == nil || verdict.Mode != ModeApproval {
		t.Errorf("approval override = %+v", verdict)
	}
	if verdict := Decide(bashInput(t, "git add ."), Policy{Rules: map[string]string{"git-add": "deny"}}); verdict == nil || verdict.Mode != ModeDeny {
		t.Errorf("deny override = %+v", verdict)
	}
}

func TestLockedRulesIgnoreOverrides(t *testing.T) {
	forced := Policy{Rules: map[string]string{"secret-path": "allow", "guard-config": "allow", "keyring": "allow", "dynamic-command": "allow"}}
	for _, command := range []string{"cat ~/.ssh/id_rsa", "echo x > ~/.claude/settings.json", "secret-tool lookup a b", "$CMD x"} {
		if verdict := Decide(bashInput(t, command), forced); verdict == nil {
			t.Errorf("%q was allowed by an override of a locked rule", command)
		}
	}
}

func TestPolicyCustomBinary(t *testing.T) {
	policy := Policy{Binaries: []string{"make"}}
	for _, command := range []string{"make build", "env CC=gcc make", "bash -c 'cd x && make'", "/usr/bin/make"} {
		verdict := Decide(bashInput(t, command), policy)
		if verdict == nil || verdict.RuleID != "custom-binary" {
			t.Errorf("%q verdict = %+v, want custom-binary", command, verdict)
		}
	}
	if verdict := Decide(bashInput(t, "go build ./..."), policy); verdict != nil {
		t.Errorf("an unrelated command was blocked: %+v", verdict)
	}
}

func TestPolicyCustomPath(t *testing.T) {
	policy := Policy{Paths: []string{"~/private", "/data/*/secret"}}
	blocked := []string{"cat ~/private/a.txt", "ls ~/private", "echo x > ~/private/new", "cat /data/one/secret/file", "cp /data/two/secret x"}
	for _, command := range blocked {
		verdict := Decide(bashInput(t, command), policy)
		if verdict == nil || verdict.RuleID != "custom-path" {
			t.Errorf("%q verdict = %+v, want custom-path", command, verdict)
		}
	}
	for _, command := range []string{"cat ~/privateer/a.txt", "ls /data/one/public", "ls ~"} {
		if verdict := Decide(bashInput(t, command), policy); verdict != nil {
			t.Errorf("%q was blocked: %+v", command, verdict)
		}
	}
	read := Input{Tool: "Read", Paths: []string{"~/private/a.txt"}, Cwd: t.TempDir(), Home: t.TempDir()}
	if verdict := Decide(read, policy); verdict == nil || verdict.RuleID != "custom-path" {
		t.Errorf("Read verdict = %+v, want custom-path", verdict)
	}
}

func TestCatalogListing(t *testing.T) {
	seen := map[string]bool{}
	for _, info := range Catalog() {
		if seen[info.ID] {
			t.Errorf("duplicate rule id %q", info.ID)
		}
		seen[info.ID] = true
		if info.Category == "Other" || info.Description == "" {
			t.Errorf("rule %q has category %q and description %q", info.ID, info.Category, info.Description)
		}
		if info.Mode != "deny" && info.Mode != "approval" {
			t.Errorf("rule %q has mode %q", info.ID, info.Mode)
		}
	}
	for _, id := range []string{"rm-force", "rm-plain", "git-add", "secret-path", "keyring"} {
		if !seen[id] {
			t.Errorf("rule %q missing from the catalog listing", id)
		}
	}
}

func TestCombineLayersGlobalUnderTheProject(t *testing.T) {
	global := Policy{Binaries: []string{"make", "cowsay"}, Paths: []string{"~/private", "/data"}, Rules: map[string]string{"git-add": "allow"}}
	project := Policy{
		Rules:           map[string]string{"git-commit": "allow"},
		Binaries:        []string{"figlet", "MAKE"},
		Paths:           []string{"~/notes"},
		ExcludeBinaries: []string{"Cowsay"},
		ExcludePaths:    []string{"/data"},
	}
	got := Combine(global, project)
	if strings.Join(got.Binaries, ",") != "make,figlet" {
		t.Errorf("binaries = %v", got.Binaries)
	}
	if strings.Join(got.Paths, ",") != "~/private,~/notes" {
		t.Errorf("paths = %v", got.Paths)
	}
	if len(got.Rules) != 1 || got.Rules["git-commit"] != "allow" {
		t.Errorf("rules = %v, want the project's only", got.Rules)
	}
	if len(got.ExcludeBinaries)+len(got.ExcludePaths) != 0 {
		t.Errorf("exclusions leaked into the combined policy: %+v", got)
	}
	if empty := Combine(global, Policy{}); strings.Join(empty.Binaries, ",") != "make,cowsay" || len(empty.Rules) != 0 {
		t.Errorf("project with nothing stored = %+v", empty)
	}
}

func TestValidateGlobalHoldsCustomRulesOnly(t *testing.T) {
	if err := (Policy{Binaries: []string{"make"}, Paths: []string{"~/x"}}).ValidateGlobal(); err != nil {
		t.Errorf("lists only = %v", err)
	}
	for _, p := range []Policy{
		{Rules: map[string]string{"git-add": "allow"}},
		{ExcludeBinaries: []string{"make"}},
		{ExcludePaths: []string{"~/x"}},
	} {
		if err := p.ValidateGlobal(); !errors.Is(err, ErrInvalidPolicy) {
			t.Errorf("ValidateGlobal(%+v) = %v", p, err)
		}
	}
	if err := (Policy{ExcludeBinaries: []string{"a/b"}}).Validate(); err == nil {
		t.Error("an exclusion with a slash passed")
	}
}
