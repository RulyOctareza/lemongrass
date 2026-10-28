// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// sealedPolicyEntryName is the policy as it was stored before it moved to policyFileName, read only to migrate it.
const sealedPolicyEntryName = "policy"

// The policy needs integrity, not secrecy, so it is stored unsealed and loaded at start: a sealed one would be absent until the human unlocks. Only ops carrying the root secret write it.
const policyFileName = "policy.json"

var ErrNoPassphrase = errors.New("vault: no passphrase is set")

// policyFile holds the global policy and each project's own, keyed by project id. A project absent from Projects uses Global.
type policyFile struct {
	Global   json.RawMessage            `json:"global,omitempty"`
	Projects map[string]json.RawMessage `json:"projects,omitempty"`
}

// loadPolicies reads the policy file. A missing or unreadable file loads as empty, which leaves every scope at the built-in catalog rather than stopping the daemon.
func loadPolicies(path string) policyFile {
	var file policyFile
	data, err := os.ReadFile(path)
	if err != nil {
		return file
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return policyFile{}
	}
	return file
}

func (s *Service) savePoliciesLocked(file policyFile) error {
	data, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("vault: encoding policies: %w", err)
	}
	if err := writeFileAtomic(s.policyPath, data, 0o600); err != nil {
		return fmt.Errorf("vault: writing %s: %w", policyFileName, err)
	}
	s.policy = file
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".policy-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (s *Service) checkPolicyWriter(rootSecret string) error {
	has, err := s.HasPassphrase()
	if err != nil {
		return err
	}
	if !has {
		return ErrNoPassphrase
	}
	return s.VerifyPassphrase(rootSecret)
}

// PutPolicy stores policy as the global one when projectID is empty, else as that project's own. The vault checks only that it is JSON.
func (s *Service) PutPolicy(rootSecret, projectID string, policy []byte) error {
	if err := s.checkPolicyWriter(rootSecret); err != nil {
		return err
	}
	if !json.Valid(policy) {
		return errors.New("vault: policy is not JSON")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.copyPoliciesLocked()
	if projectID == "" {
		next.Global = append(json.RawMessage(nil), policy...)
	} else {
		if next.Projects == nil {
			next.Projects = map[string]json.RawMessage{}
		}
		next.Projects[projectID] = append(json.RawMessage(nil), policy...)
	}
	return s.savePoliciesLocked(next)
}

// PolicyParts returns the global policy and projectID's own, either empty when none is stored. The caller combines them.
func (s *Service) PolicyParts(projectID string) (global, project []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if projectID != "" {
		project = append([]byte(nil), s.policy.Projects[projectID]...)
	}
	return append([]byte(nil), s.policy.Global...), project
}

// KeepProjectPolicies drops every project policy whose id is not in keep, writing the file only when something was dropped. It needs no root secret: the caller passes the project registry, which agents cannot edit.
func (s *Service) KeepProjectPolicies(keep map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.copyPoliciesLocked()
	dropped := false
	for id := range next.Projects {
		if !keep[id] {
			delete(next.Projects, id)
			dropped = true
		}
	}
	if !dropped {
		return nil
	}
	return s.savePoliciesLocked(next)
}

// Policies returns the global policy and every project's own, for the human-facing editor.
func (s *Service) Policies() (global []byte, projects map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	projects = make(map[string][]byte, len(s.policy.Projects))
	for id, p := range s.policy.Projects {
		projects[id] = append([]byte(nil), p...)
	}
	return append([]byte(nil), s.policy.Global...), projects
}

// MigrateSealedPolicy moves a policy sealed by an earlier version into the policy file as the global policy, unless one is already there, then deletes the sealed entry.
func (s *Service) MigrateSealedPolicy(rootSecret string) error {
	if err := s.VerifyPassphrase(rootSecret); err != nil {
		return err
	}
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)
	sealed, err := s.policies.Get(sealedPolicyEntryName, rootKey)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.policy.Global) == 0 && len(sealed) > 0 && json.Valid(sealed) {
		next := s.copyPoliciesLocked()
		next.Global = sealed
		if err := s.savePoliciesLocked(next); err != nil {
			return err
		}
	}
	return s.policies.Delete(sealedPolicyEntryName)
}

func (s *Service) copyPoliciesLocked() policyFile {
	next := policyFile{Global: s.policy.Global}
	if len(s.policy.Projects) > 0 {
		next.Projects = make(map[string]json.RawMessage, len(s.policy.Projects))
		for id, p := range s.policy.Projects {
			next.Projects[id] = p
		}
	}
	return next
}
