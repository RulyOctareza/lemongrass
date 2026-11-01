// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/vault"
)

// Ask ops. create_ask, wait_ask, ask_result and ack_ask are the model's, filed through the lgrass binary and refused to any other peer. The rest are the app's: ask_count carries no ask content and needs no secret, every other one carries the root secret, so no lgrass subcommand can answer an ask.
const (
	opCreateAsk  = "create_ask"
	opWaitAsk    = "wait_ask"
	opAskResult  = "ask_result"
	opAckAsk     = "ack_ask"
	opAskCount   = "ask_count"
	opListAsks   = "list_asks"
	opGetAsk     = "get_ask"
	opAnswerAsk  = "answer_ask"
	opExecuteAsk = "execute_ask"
	opStopAsk    = "stop_ask"
	opDeclineAsk = "decline_ask"
)

// waitAskSlack is how much longer than the wait itself the wait_ask connection stays open, for the reply to be written.
const waitAskSlack = 30 * time.Second

type createAskPayload struct {
	Type    string   `json:"type"`
	TabID   string   `json:"tab_id"`
	Cwd     string   `json:"cwd"`
	Path    string   `json:"path,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Tables  []string `json:"tables,omitempty"`
	Message string   `json:"message,omitempty"`
	Command string   `json:"command,omitempty"`
	Rule    string   `json:"rule,omitempty"`
}

type waitAskPayload struct {
	ID             string `json:"id"`
	TabID          string `json:"tab_id"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type askCountPayload struct {
	Waiting  int    `json:"waiting"`
	Blocking int    `json:"blocking"`
	NewestID string `json:"newest_id"`
}

type askIDPayload struct {
	RootSecret string `json:"root_secret"`
	ID         string `json:"id"`
}

type answerAskPayload struct {
	RootSecret string `json:"root_secret"`
	ID         string `json:"id"`
	ShortID    string `json:"short_id"`
}

type executeAskPayload struct {
	RootSecret     string `json:"root_secret"`
	ID             string `json:"id"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type declineAskPayload struct {
	RootSecret string `json:"root_secret"`
	ID         string `json:"id"`
	Note       string `json:"note"`
}

// AskVerdict is the guard's verdict for a command ask's command, shown on the card. Mode is ask or deny.
type AskVerdict struct {
	Rule        string `json:"rule"`
	Description string `json:"description,omitempty"`
	Mode        string `json:"mode"`
}

// AskView is an ask as the app sees it: the record plus what the card needs to judge a command.
type AskView struct {
	vault.Ask
	Verdict            *AskVerdict `json:"verdict"`
	NeverExecute       bool        `json:"never_execute"`
	NeverExecuteReason string      `json:"never_execute_reason,omitempty"`
}

type askPayload struct {
	Ask AskView `json:"ask"`
}

type askListPayload struct {
	Asks []AskView `json:"asks"`
}

// modelAskPayload is an ask as the model reads it back.
type modelAskPayload struct {
	Ask vault.Ask `json:"ask"`
}

func (b *Backend) askView(a vault.Ask) AskView {
	view := AskView{Ask: a}
	if a.Type != vault.AskCommand {
		return view
	}
	home, _ := os.UserHomeDir()
	view.NeverExecuteReason, view.NeverExecute = guard.NeverExecute(a.Command, a.Cwd, home)
	proj, _ := project.Resolve(a.Cwd)
	verdict := guard.Decide(guard.Input{Tool: "Bash", Command: a.Command, Cwd: a.Cwd, Home: home, ProjectRoot: proj.Path}, b.EffectivePolicy(proj.ID))
	if verdict != nil {
		mode := "ask"
		if verdict.Mode == guard.ModeDeny {
			mode = "deny"
		}
		view.Verdict = &AskVerdict{Rule: verdict.RuleID, Description: verdict.Description, Mode: mode}
	}
	return view
}

func (b *Backend) verifyRoot(rootSecret string) error {
	return b.Service.VerifyPassphrase(rootSecret)
}

// CreateChannelForAsk creates a db channel for a waiting db ask, refusing a connection that lacks any table the model asked for, and links the channel to the ask.
func (b *Backend) CreateChannelForAsk(askID, rootSecret, name, dbName string, scope vault.Scope, ttl time.Duration) (vault.Channel, error) {
	ask, err := b.Asks.CheckChannelAsk(askID, vault.AskDB)
	if err != nil {
		return vault.Channel{}, err
	}
	have, err := b.DB.ListTables(rootSecret, dbName)
	if err != nil {
		return vault.Channel{}, err
	}
	if missing := missingTables(ask.Tables, have); len(missing) > 0 {
		return vault.Channel{}, fmt.Errorf("gatekeeper: connection %s has no table %s, which the ask requests", dbName, strings.Join(missing, ", "))
	}
	c, err := b.Service.CreateChannel(rootSecret, name, dbName, scope, ttl)
	if err != nil {
		return vault.Channel{}, err
	}
	if err := b.Asks.LinkChannel(askID, vault.AskDB, c.ID); err != nil {
		return c, err
	}
	return c, nil
}

// CreateHTTPChannelForAsk creates an HTTP channel for a waiting rester ask and links it to the ask. A rester ask is free text, so there is nothing to check the channel against.
func (b *Backend) CreateHTTPChannelForAsk(askID, rootSecret, name, rester, environment, typ string, scope vault.HTTPScope, webScope *vault.WebScope, ttl time.Duration) (vault.HTTPChannel, error) {
	if _, err := b.Asks.CheckChannelAsk(askID, vault.AskRester); err != nil {
		return vault.HTTPChannel{}, err
	}
	c, err := b.Service.CreateHTTPChannel(rootSecret, name, rester, environment, typ, scope, webScope, ttl)
	if err != nil {
		return vault.HTTPChannel{}, err
	}
	if err := b.Asks.LinkChannel(askID, vault.AskRester, c.ID); err != nil {
		return c, err
	}
	return c, nil
}

// missingTables lists the requested tables the connection lacks. Names compare without case, since MySQL on some platforms and Postgres unquoted identifiers fold case.
func missingTables(requested, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, t := range have {
		present[strings.ToLower(t)] = true
	}
	var missing []string
	for _, t := range requested {
		if !present[strings.ToLower(t)] {
			missing = append(missing, t)
		}
	}
	return missing
}

// serveWaitAsk blocks on the ask for up to the requested wait and writes the ask back. A failed write sends the outcome to the tab as a nudge at once; a successful one still waits for lgrass's ack_ask, since the reply may pass through the agent.
func serveWaitAsk(svc *Backend, conn net.Conn, req request) {
	var p waitAskPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		json.NewEncoder(conn).Encode(errResponse(err))
		return
	}
	wait := time.Duration(p.TimeoutSeconds) * time.Second
	if wait <= 0 || wait > vault.AskMaxWait {
		wait = vault.AskMaxWait
	}
	conn.SetDeadline(time.Now().Add(wait + waitAskSlack))
	ask, err := svc.Asks.Wait(p.ID, p.TabID, wait)
	if err != nil {
		json.NewEncoder(conn).Encode(errResponse(err))
		return
	}
	writeErr := json.NewEncoder(conn).Encode(payloadResponse(modelAskPayload{Ask: ask}))
	if ask.Final() && writeErr != nil {
		svc.Asks.Delivered(ask.ID, false)
	}
}

// dispatchAsk handles the ask ops and reports whether op was one of them.
func dispatchAsk(svc *Backend, adminLimiter *vault.FailureLimiter, req request) (response, bool) {
	switch req.Op {
	case opCreateAsk:
		var p createAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		ask, err := svc.Asks.Create(vault.AskInput{
			Type: p.Type, TabID: p.TabID, Cwd: p.Cwd, Path: p.Path, Reason: p.Reason,
			Tables: p.Tables, Message: p.Message, Command: p.Command, Rule: p.Rule,
		})
		if err != nil {
			return errResponse(err), true
		}
		return payloadResponse(modelAskPayload{Ask: ask}), true

	case opAskResult:
		var p waitAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		ask, err := svc.Asks.Result(p.ID, p.TabID)
		if err != nil {
			return errResponse(err), true
		}
		if ask.Final() {
			svc.Asks.Delivered(ask.ID, true)
		}
		return payloadResponse(modelAskPayload{Ask: ask}), true

	case opAckAsk:
		var p waitAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		if err := svc.Asks.Ack(p.ID, p.TabID); err != nil {
			return errResponse(err), true
		}
		return response{OK: true}, true

	case opAskCount:
		waiting, blocking, newest := svc.Asks.Count()
		return payloadResponse(askCountPayload{Waiting: waiting, Blocking: blocking, NewestID: newest}), true

	case opListAsks:
		var p rootSecretPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			views := []AskView{}
			for _, a := range svc.Asks.List() {
				views = append(views, svc.askView(a))
			}
			return payloadResponse(askListPayload{Asks: views}), nil
		}), true

	case opGetAsk:
		var p askIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			a, err := svc.Asks.Get(p.ID)
			if err != nil {
				return response{}, err
			}
			return payloadResponse(askPayload{Ask: svc.askView(a)}), nil
		}), true

	case opAnswerAsk:
		var p answerAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			return response{OK: true}, svc.Asks.AnswerChannel(p.ID, p.ShortID)
		}), true

	case opExecuteAsk:
		var p executeAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			a, err := svc.ExecuteAsk(p.ID, time.Duration(p.TimeoutSeconds)*time.Second)
			if err != nil {
				return response{}, err
			}
			return payloadResponse(askPayload{Ask: svc.askView(a)}), nil
		}), true

	case opStopAsk:
		var p askIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			return response{OK: true}, svc.StopAsk(p.ID)
		}), true

	case opDeclineAsk:
		var p declineAskPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		return adminOp(adminLimiter, func() (response, error) {
			if err := svc.verifyRoot(p.RootSecret); err != nil {
				return response{}, err
			}
			return response{OK: true}, svc.Asks.Decline(p.ID, p.Note)
		}), true
	}
	return response{}, false
}

// AskRequest is what the model files, with the caller's tab, directory and PATH filled in by the caller.
type AskRequest = createAskPayload

// CreateAsk files an ask and returns it at once.
func (c *Client) CreateAsk(req AskRequest) (vault.Ask, error) {
	var out modelAskPayload
	err := c.call(opCreateAsk, req, &out)
	return out.Ask, err
}

// WaitAsk blocks until the ask has its outcome or wait passes, and returns it as it then stands.
func (c *Client) WaitAsk(id, tabID string, wait time.Duration) (vault.Ask, error) {
	var out modelAskPayload
	err := c.callWithin(wait+waitAskSlack, opWaitAsk, waitAskPayload{ID: id, TabID: tabID, TimeoutSeconds: int(wait / time.Second)}, &out)
	return out.Ask, err
}

// AckAsk tells the vault the model has the ask's outcome, so no nudge repeats it.
func (c *Client) AckAsk(id, tabID string) error {
	return c.call(opAckAsk, waitAskPayload{ID: id, TabID: tabID}, nil)
}

// AskResult reads an ask the tab filed.
func (c *Client) AskResult(id, tabID string) (vault.Ask, error) {
	var out modelAskPayload
	err := c.call(opAskResult, waitAskPayload{ID: id, TabID: tabID}, &out)
	return out.Ask, err
}

// The app's ask ops have Go clients only for tests and tooling; the app calls the socket itself.

func (c *Client) AskCount() (waiting, blocking int, newestID string, err error) {
	var out askCountPayload
	err = c.call(opAskCount, nil, &out)
	return out.Waiting, out.Blocking, out.NewestID, err
}

func (c *Client) ListAsks(rootSecret string) ([]AskView, error) {
	var out askListPayload
	err := c.call(opListAsks, rootSecretPayload{RootSecret: rootSecret}, &out)
	return out.Asks, err
}

func (c *Client) GetAsk(rootSecret, id string) (AskView, error) {
	var out askPayload
	err := c.call(opGetAsk, askIDPayload{RootSecret: rootSecret, ID: id}, &out)
	return out.Ask, err
}

func (c *Client) AnswerAsk(rootSecret, id, shortID string) error {
	return c.call(opAnswerAsk, answerAskPayload{RootSecret: rootSecret, ID: id, ShortID: shortID}, nil)
}

func (c *Client) ExecuteAsk(rootSecret, id string, timeout time.Duration) (AskView, error) {
	var out askPayload
	err := c.call(opExecuteAsk, executeAskPayload{RootSecret: rootSecret, ID: id, TimeoutSeconds: int(timeout / time.Second)}, &out)
	return out.Ask, err
}

func (c *Client) StopAsk(rootSecret, id string) error {
	return c.call(opStopAsk, askIDPayload{RootSecret: rootSecret, ID: id}, nil)
}

func (c *Client) DeclineAsk(rootSecret, id, note string) error {
	return c.call(opDeclineAsk, declineAskPayload{RootSecret: rootSecret, ID: id, Note: note}, nil)
}

func (c *Client) CreateChannelForAsk(rootSecret, askID, name, dbName string, scope vault.Scope, ttl time.Duration) (vault.Channel, error) {
	var out channelPayload
	err := c.call(opCreateChannel, createChannelPayload{RootSecret: rootSecret, AskID: askID, Name: name, DBName: dbName, Scope: scope, TTLSeconds: int(ttl / time.Second)}, &out)
	return out.Channel, err
}

func (c *Client) CreateHTTPChannelForAsk(rootSecret, askID, name, rester, environment, typ string, scope vault.HTTPScope, webScope *vault.WebScope, ttl time.Duration) (vault.HTTPChannel, error) {
	var out httpChannelPayload
	err := c.call(opCreateHTTPChannel, createHTTPChannelPayload{RootSecret: rootSecret, AskID: askID, Name: name, Rester: rester, Environment: environment, Type: typ, Scope: scope, WebScope: webScope, TTLSeconds: int(ttl / time.Second)}, &out)
	return out.Channel, err
}
