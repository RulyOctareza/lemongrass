// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package gitconflict is the Go side of handing a git conflict to a model tab: the app socket ops it calls and the few git reads it needs. Reading conflicts themselves stays in the app's main process.
package gitconflict

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/faizalv/lemongrass/internal/ipc"
	"time"
)

const (
	opShow   = "conflict_show"
	opSubmit = "conflict_submit"
	opClaim  = "conflict_claim"

	ActionStart = "start"
	ActionPost  = "post"
	ActionClose = "close"

	dialTimeout = time.Second
	callTimeout = 30 * time.Second
)

type request struct {
	Op          string   `json:"op"`
	ProjectPath string   `json:"project_path,omitempty"`
	TabID       string   `json:"tab_id,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	ThreadID    string   `json:"thread_id,omitempty"`
	Files       []string `json:"files,omitempty"`
	Token       string   `json:"token,omitempty"`
}

type response struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Text     string `json:"text,omitempty"`
	Conflict *Grant `json:"conflict,omitempty"`
}

type Anchor struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Quote     string `json:"quote"`
}

type HumanMessage struct {
	Body   string  `json:"body"`
	Anchor *Anchor `json:"anchor"`
}

// Grant is the one write a claimed token authorises, minted by the app when the human acted.
type Grant struct {
	Action      string         `json:"action"`
	ProjectPath string         `json:"project_path"`
	Scope       string         `json:"scope"`
	TabID       string         `json:"tab_id,omitempty"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Messages    []HumanMessage `json:"messages,omitempty"`
}

// ShowRequest names the conflict a tab asks the app to describe.
type ShowRequest struct {
	ProjectPath string
	TabID       string
	Scope       string
	ThreadID    string
}

// Show asks the app for the text describing the conflict: its operation, refs, files and blocks, and both sides' commit intent.
func Show(socketPath string, req ShowRequest) (string, error) {
	resp, err := call(socketPath, request{Op: opShow, ProjectPath: req.ProjectPath, TabID: req.TabID, Scope: req.Scope, ThreadID: req.ThreadID})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// Submit tells the app the tab submitted files, so it opens the resolver on them.
func Submit(socketPath, projectPath, tabID, scope string, files []string) error {
	_, err := call(socketPath, request{Op: opSubmit, ProjectPath: projectPath, TabID: tabID, Scope: scope, Files: files})
	return err
}

// Claim redeems a token the app minted, once. Only the app mints one, so a model running the app command itself gets nothing.
func Claim(socketPath, token string) (Grant, error) {
	resp, err := call(socketPath, request{Op: opClaim, Token: token})
	if err != nil {
		return Grant{}, err
	}
	if resp.Conflict == nil {
		return Grant{}, errors.New("gitconflict: the app returned no action for the token")
	}
	switch resp.Conflict.Action {
	case ActionStart, ActionPost, ActionClose:
		return *resp.Conflict, nil
	}
	return Grant{}, fmt.Errorf("gitconflict: the app returned the unknown action %q", resp.Conflict.Action)
}

func call(socketPath string, req request) (response, error) {
	conn, err := ipc.Dial(socketPath, dialTimeout)
	if err != nil {
		return response{}, fmt.Errorf("gitconflict: the lemongrass app is not reachable at %s, is it running: %w", socketPath, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(callTimeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response{}, fmt.Errorf("gitconflict: sending to the app: %w", err)
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return response{}, fmt.Errorf("gitconflict: reading the app's answer: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			return response{}, errors.New("gitconflict: the app refused the request")
		}
		return response{}, errors.New("gitconflict: " + resp.Error)
	}
	return resp, nil
}
