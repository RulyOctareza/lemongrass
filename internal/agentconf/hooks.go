// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

var ClaudeHookEvents = []string{"SessionStart", "SessionEnd", "PreToolUse", "PostToolUse", "UserPromptSubmit", "Notification", "Stop"}

type HookHandler struct {
	Type                   string `json:"type"`
	Command                string `json:"command"`
	Timeout                int    `json:"timeout,omitempty"`
	StatusMessage          string `json:"statusMessage,omitempty"`
	AdditionalContextLimit int    `json:"additionalContextLimit,omitempty"`
}

type HookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []HookHandler `json:"hooks"`
}

type HookRegistration struct {
	Event   string
	Matcher string
	Handler HookHandler
}

func ownedByEvent(group json.RawMessage, event string) bool {
	var g struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(group, &g) != nil {
		return false
	}
	suffix := " hook " + event
	for _, h := range g.Hooks {
		if strings.HasSuffix(h.Command, suffix) {
			return true
		}
	}
	return false
}

func sameJSON(a, b []byte) bool {
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return false
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

func ReconcileClaudeSettings(data []byte, lgrassdPath string) ([]byte, bool, error) {
	registrations := make([]HookRegistration, 0, len(ClaudeHookEvents))
	for _, event := range ClaudeHookEvents {
		registrations = append(registrations, HookRegistration{
			Event:   event,
			Handler: HookHandler{Type: "command", Command: lgrassdPath + " hook " + event},
		})
	}
	return ReconcileHookGroups(data, registrations)
}

// Returns the input untouched and false when every Lemongrass hook group already matches.
func ReconcileHookGroups(data []byte, registrations []HookRegistration) ([]byte, bool, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, false, fmt.Errorf("parsing settings: %w", err)
		}
	}

	hooks := map[string][]json.RawMessage{}
	if raw, ok := top["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, false, fmt.Errorf("parsing hooks: %w", err)
		}
	}

	dirty := false
	for _, registration := range registrations {
		event := registration.Event
		desired, err := json.Marshal(HookGroup{Matcher: registration.Matcher, Hooks: []HookHandler{registration.Handler}})
		if err != nil {
			return nil, false, err
		}

		var groups []json.RawMessage
		placed := false
		for _, g := range hooks[event] {
			if !ownedByEvent(g, event) {
				groups = append(groups, g)
				continue
			}
			if placed {
				dirty = true
				continue
			}
			placed = true
			if !sameJSON(g, desired) {
				dirty = true
			}
			groups = append(groups, desired)
		}
		if !placed {
			dirty = true
			groups = append(groups, desired)
		}
		hooks[event] = groups
	}
	if !dirty {
		return data, false, nil
	}

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return nil, false, err
	}
	top["hooks"] = hooksJSON
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// RemoveHookGroups drops every Lemongrass hook group of the given events, found by the same rule reconcile uses to
// claim them, then event lists left empty and a hooks object left empty. empty reports that nothing else is left in
// the settings, so a file Lemongrass created can go.
func RemoveHookGroups(data []byte, events []string) (out []byte, changed, empty bool, err error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, false, false, fmt.Errorf("agentconf: parsing settings: %w", err)
		}
	}
	raw, ok := top["hooks"]
	if !ok {
		return data, false, len(top) == 0, nil
	}
	hooks := map[string][]json.RawMessage{}
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return nil, false, false, fmt.Errorf("agentconf: parsing hooks: %w", err)
	}
	for _, event := range events {
		groups, ok := hooks[event]
		if !ok {
			continue
		}
		var kept []json.RawMessage
		for _, g := range groups {
			if ownedByEvent(g, event) {
				changed = true
				continue
			}
			kept = append(kept, g)
		}
		if len(kept) == 0 {
			changed = true
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !changed {
		return data, false, false, nil
	}
	if len(hooks) == 0 {
		delete(top, "hooks")
	} else {
		hooksJSON, err := json.Marshal(hooks)
		if err != nil {
			return nil, false, false, err
		}
		top["hooks"] = hooksJSON
	}
	out, err = json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, false, false, err
	}
	return append(out, '\n'), true, len(top) == 0, nil
}
