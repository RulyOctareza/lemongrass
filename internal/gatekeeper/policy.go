// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"fmt"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
)

const (
	opPutPolicy     = "put_policy"
	opListPolicies  = "list_policies"
	opMigratePolicy = "migrate_policy"
	opCurrentPolicy = "current_policy"
	opPolicyCatalog = "policy_catalog"
)

type putPolicyPayload struct {
	RootSecret string          `json:"root_secret"`
	ProjectID  string          `json:"project_id,omitempty"`
	Policy     json.RawMessage `json:"policy"`
}

type currentPolicyPayload struct {
	ProjectID string `json:"project_id,omitempty"`
}

type policyPayload struct {
	Policy json.RawMessage `json:"policy,omitempty"`
}

type policiesPayload struct {
	Global   json.RawMessage            `json:"global,omitempty"`
	Projects map[string]json.RawMessage `json:"projects,omitempty"`
}

type policyCatalogPayload struct {
	Rules []guard.RuleInfo `json:"rules"`
}

func registeredProject(id string) error {
	projects, err := project.List()
	if err != nil {
		return err
	}
	for _, p := range projects {
		if p.ID == id {
			return nil
		}
	}
	return fmt.Errorf("gatekeeper: project %q is not registered", id)
}

// pruneProjectPolicies drops the policies of projects that left the registry. A registry that cannot be read or lists no project prunes nothing, so a broken registry never wipes every project's policy.
func (b *Backend) pruneProjectPolicies() error {
	projects, err := project.List()
	if err != nil || len(projects) == 0 {
		return nil
	}
	keep := make(map[string]bool, len(projects))
	for _, p := range projects {
		keep[p.ID] = true
	}
	return b.Service.KeepProjectPolicies(keep)
}

// PutPolicy validates policyJSON against the guard catalog before the vault stores it, so a locked rule or a blocked lgrass binary is refused here, not only in the UI.
func (b *Backend) PutPolicy(rootSecret, projectID string, policyJSON []byte) error {
	policy, err := guard.ParsePolicy(policyJSON)
	if err != nil {
		return err
	}
	if projectID == "" {
		if err := policy.ValidateGlobal(); err != nil {
			return err
		}
	} else if err := registeredProject(projectID); err != nil {
		return err
	}
	normalized, err := policy.Marshal()
	if err != nil {
		return err
	}
	if err := b.Service.PutPolicy(rootSecret, projectID, normalized); err != nil {
		return err
	}
	return b.pruneProjectPolicies()
}

// PutPolicy stores policy as the global one when projectID is empty, else as that project's own.
func (c *Client) PutPolicy(rootSecret, projectID string, policy guard.Policy) error {
	raw, err := policy.Marshal()
	if err != nil {
		return err
	}
	return c.call(opPutPolicy, putPolicyPayload{RootSecret: rootSecret, ProjectID: projectID, Policy: raw}, nil)
}

// Policies returns the global policy and every project's own. It needs no root secret, since the policy is not secret.
func (c *Client) Policies() (guard.Policy, map[string]guard.Policy, error) {
	var out policiesPayload
	if err := c.call(opListPolicies, nil, &out); err != nil {
		return guard.Policy{}, nil, err
	}
	global, err := guard.ParsePolicy(out.Global)
	if err != nil {
		return guard.Policy{}, nil, err
	}
	projects := make(map[string]guard.Policy, len(out.Projects))
	for id, raw := range out.Projects {
		p, err := guard.ParsePolicy(raw)
		if err != nil {
			return guard.Policy{}, nil, err
		}
		projects[id] = p
	}
	return global, projects, nil
}

func (c *Client) MigratePolicy(rootSecret string) error {
	return c.call(opMigratePolicy, rootSecretPayload{RootSecret: rootSecret}, nil)
}

// EffectivePolicy combines the global policy with projectID's own. A scope that fails validation counts as empty, so one bad scope never takes the other down with it.
func (b *Backend) EffectivePolicy(projectID string) guard.Policy {
	globalRaw, projectRaw := b.Service.PolicyParts(projectID)
	global, err := guard.ParsePolicy(globalRaw)
	if err != nil {
		global = guard.Policy{}
	}
	project, err := guard.ParsePolicy(projectRaw)
	if err != nil {
		project = guard.Policy{}
	}
	return guard.Combine(global, project)
}

// CurrentPolicy returns the policy the hook enforces for projectID, global and project combined.
func (c *Client) CurrentPolicy(projectID string) (guard.Policy, error) {
	var out policyPayload
	if err := c.call(opCurrentPolicy, currentPolicyPayload{ProjectID: projectID}, &out); err != nil {
		return guard.Policy{}, err
	}
	return guard.ParsePolicy(out.Policy)
}

func (c *Client) PolicyCatalog() ([]guard.RuleInfo, error) {
	var out policyCatalogPayload
	err := c.call(opPolicyCatalog, nil, &out)
	return out.Rules, err
}
