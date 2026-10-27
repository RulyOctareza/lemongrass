// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"errors"
	"testing"
)

func newPolicyService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassphrase("correct horse"); err != nil {
		t.Fatal(err)
	}
	return svc, dir
}

func TestPolicySurvivesRestartWithoutUnlock(t *testing.T) {
	svc, dir := newPolicyService(t)
	if got := global(svc); len(got) != 0 {
		t.Fatalf("policy before any was stored = %q", got)
	}
	if err := svc.PutPolicy("correct horse", "", []byte(`{"binaries":["make"]}`)); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	reopened, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(global(reopened)); got != `{"binaries":["make"]}` {
		t.Errorf("policy after restart = %q", got)
	}
}

func global(svc *Service) []byte {
	g, _ := svc.PolicyParts("")
	return g
}

func TestPolicyPartsKeepScopesApart(t *testing.T) {
	svc, _ := newPolicyService(t)
	if err := svc.PutPolicy("correct horse", "", []byte(`{"binaries":["make"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutPolicy("correct horse", "p1", []byte(`{"binaries":["cargo"]}`)); err != nil {
		t.Fatal(err)
	}
	g, p := svc.PolicyParts("p1")
	if string(g) != `{"binaries":["make"]}` || string(p) != `{"binaries":["cargo"]}` {
		t.Errorf("PolicyParts(p1) = %q, %q", g, p)
	}
	if _, p := svc.PolicyParts("p2"); len(p) != 0 {
		t.Errorf("project with nothing stored = %q", p)
	}
	if _, p := svc.PolicyParts(""); len(p) != 0 {
		t.Errorf("no project = %q", p)
	}
	all, projects := svc.Policies()
	if string(all) != `{"binaries":["make"]}` || len(projects) != 1 || string(projects["p1"]) != `{"binaries":["cargo"]}` {
		t.Errorf("Policies = %q, %q", all, projects)
	}
}

func TestPolicyWritesNeedThePassphrase(t *testing.T) {
	svc, _ := newPolicyService(t)
	if err := svc.PutPolicy("wrong", "", []byte(`{}`)); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("PutPolicy with a wrong passphrase = %v", err)
	}
	if err := svc.PutPolicy("correct horse", "", []byte(`not json`)); err == nil {
		t.Error("PutPolicy accepted bytes that are not JSON")
	}

	fresh, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.PutPolicy("x", "", []byte(`{}`)); !errors.Is(err, ErrNoPassphrase) {
		t.Errorf("PutPolicy with no passphrase set = %v", err)
	}
}

func TestMigrateSealedPolicy(t *testing.T) {
	svc, dir := newPolicyService(t)
	rootKey, err := DeriveKey("correct horse", svc.rootSalt)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.policies.Put(sealedPolicyEntryName, rootKey, []byte(`{"rules":{"git-add":"allow"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.MigrateSealedPolicy("wrong"); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("migrate with a wrong passphrase = %v", err)
	}
	if err := svc.MigrateSealedPolicy("correct horse"); err != nil {
		t.Fatalf("MigrateSealedPolicy: %v", err)
	}
	reopened, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(global(reopened)); got != `{"rules":{"git-add":"allow"}}` {
		t.Errorf("migrated policy = %q", got)
	}
	if names, _ := svc.policies.List(); len(names) != 0 {
		t.Errorf("sealed entry survived migration: %v", names)
	}
	if err := svc.MigrateSealedPolicy("correct horse"); err != nil {
		t.Errorf("second migration = %v", err)
	}
}

func TestMigrateKeepsANewerGlobalPolicy(t *testing.T) {
	svc, _ := newPolicyService(t)
	rootKey, err := DeriveKey("correct horse", svc.rootSalt)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.policies.Put(sealedPolicyEntryName, rootKey, []byte(`{"binaries":["old"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutPolicy("correct horse", "", []byte(`{"binaries":["new"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.MigrateSealedPolicy("correct horse"); err != nil {
		t.Fatal(err)
	}
	if got := string(global(svc)); got != `{"binaries":["new"]}` {
		t.Errorf("global policy after migration = %q", got)
	}
}

func TestResetVaultKeepsPolicy(t *testing.T) {
	svc, _ := newPolicyService(t)
	if err := svc.PutPolicy("correct horse", "", []byte(`{"binaries":["make"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetVault(); err != nil {
		t.Fatal(err)
	}
	if got := string(global(svc)); got != `{"binaries":["make"]}` {
		t.Errorf("policy after reset = %q", got)
	}
}

func TestKeepProjectPolicies(t *testing.T) {
	svc, dir := newPolicyService(t)
	for _, id := range []string{"p1", "p2"} {
		if err := svc.PutPolicy("correct horse", id, []byte(`{"binaries":["make"]}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.KeepProjectPolicies(map[string]bool{"p1": true}); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, projects := reopened.Policies()
	if len(projects) != 1 || projects["p1"] == nil {
		t.Errorf("projects after pruning = %q", projects)
	}
}
