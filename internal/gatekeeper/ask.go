// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"errors"
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
	Type       string   `json:"type"`
	TabID      string   `json:"tab_id"`
	Cwd        string   `json:"cwd"`
	Path       string   `json:"path,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Tables     []string `json:"tables,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Message    string   `json:"message,omitempty"`
	Methods    []string `json:"methods,omitempty"`
	Command    string   `json:"command,omitempty"`
	Rule       string   `json:"rule,omitempty"`
	// AccountEnv is the caller's account variables (vault.AskAccountEnvKeys).
	AccountEnv map[string]string `json:"account_env,omitempty"`
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
	// Rows and Cols are the card terminal's size, so the command starts at it.
	Rows int `json:"rows,omitempty"`
	Cols int `json:"cols,omitempty"`
}

type declineAskPayload struct {
	RootSecret string `json:"root_secret"`
	ID         string `json:"id"`
	Note       string `json:"note"`
	// DBName and RemovedTables carry the tables the human removed from a db ask before declining it.
	DBName        string   `json:"db_name,omitempty"`
	RemovedTables []string `json:"removed_tables,omitempty"`
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

// CreateChannelForAsk creates a db channel for a waiting db ask. The human may drop requested tables and declared operations but never add any, every granted table must exist in the connection, and each requested table left out is recorded with why, for the model's answer.
func (b *Backend) CreateChannelForAsk(askID, rootSecret, name, dbName string, scope vault.Scope, ttl time.Duration) (vault.Channel, error) {
	ask, err := b.Asks.CheckChannelAsk(askID, vault.AskDB)
	if err != nil {
		return vault.Channel{}, err
	}
	if len(scope.Tables) == 0 {
		return vault.Channel{}, errors.New("gatekeeper: grant at least one table, or decline the ask")
	}
	if extra := notDeclared(scope.Tables, ask.Tables); len(extra) > 0 {
		return vault.Channel{}, fmt.Errorf("gatekeeper: the ask did not request table %s; tables can be removed from an ask, not added", strings.Join(extra, ", "))
	}
	if extra := notDeclared(scope.Operations, ask.Operations); len(extra) > 0 {
		return vault.Channel{}, fmt.Errorf("gatekeeper: the ask did not declare operation %s; operations can be removed from an ask, not added", strings.Join(extra, ", "))
	}
	have, err := b.connectionTables(rootSecret, dbName)
	if err != nil {
		return vault.Channel{}, err
	}
	if missing := notDeclared(scope.Tables, have); len(missing) > 0 {
		return vault.Channel{}, fmt.Errorf("gatekeeper: connection %s has no table %s", dbName, strings.Join(missing, ", "))
	}
	removed := removedTables(notDeclared(ask.Tables, scope.Tables), dbName, have)
	c, err := b.Service.CreateChannel(rootSecret, name, dbName, scope, ttl)
	if err != nil {
		return vault.Channel{}, err
	}
	grant := vault.AskGrant{Tables: append([]string(nil), scope.Tables...), Operations: append([]string(nil), scope.Operations...)}
	if err := b.Asks.LinkChannel(askID, vault.AskDB, c.ID, grant, removed); err != nil {
		return c, err
	}
	return c, nil
}

// CreateHTTPChannelForAsk creates an HTTP channel for a waiting rester ask and links it to the ask. The human may drop declared methods but never add any.
func (b *Backend) CreateHTTPChannelForAsk(askID, rootSecret, name, rester, environment, typ string, scope vault.HTTPScope, webScope *vault.WebScope, ttl time.Duration) (vault.HTTPChannel, error) {
	ask, err := b.Asks.CheckChannelAsk(askID, vault.AskRester)
	if err != nil {
		return vault.HTTPChannel{}, err
	}
	methods := make([]string, len(scope.Methods))
	for i, m := range scope.Methods {
		methods[i] = strings.ToUpper(m)
	}
	if extra := notDeclared(methods, ask.Methods); len(extra) > 0 {
		return vault.HTTPChannel{}, fmt.Errorf("gatekeeper: the ask did not declare method %s; methods can be removed from an ask, not added", strings.Join(extra, ", "))
	}
	c, err := b.Service.CreateHTTPChannel(rootSecret, name, rester, environment, typ, scope, webScope, ttl)
	if err != nil {
		return vault.HTTPChannel{}, err
	}
	if err := b.Asks.LinkChannel(askID, vault.AskRester, c.ID, vault.AskGrant{Methods: methods}, nil); err != nil {
		return c, err
	}
	return c, nil
}

// DeclineAsk declines an ask with the human's note. For a db ask, removed names requested tables the human took out before declining; with dbName set their reasons come from that connection's tables.
func (b *Backend) DeclineAsk(rootSecret, id, note, dbName string, removed []string) error {
	var reasons []vault.RemovedTable
	if len(removed) > 0 {
		ask, err := b.Asks.Get(id)
		if err != nil {
			return err
		}
		if ask.Type != vault.AskDB {
			return fmt.Errorf("gatekeeper: ask %s is a %s ask, only a db ask has tables to remove", id, ask.Type)
		}
		if extra := notDeclared(removed, ask.Tables); len(extra) > 0 {
			return fmt.Errorf("gatekeeper: the ask did not request table %s", strings.Join(extra, ", "))
		}
		var have []string
		if dbName != "" {
			if have, err = b.connectionTables(rootSecret, dbName); err != nil {
				return err
			}
		}
		reasons = removedTables(removed, dbName, have)
	}
	return b.Asks.Decline(id, note, reasons)
}

// connectionTables lists a connection's tables; tests replace the lister, since a real listing needs a live database.
func (b *Backend) connectionTables(rootSecret, dbName string) ([]string, error) {
	if b.listTables != nil {
		return b.listTables(rootSecret, dbName)
	}
	return b.DB.ListTables(rootSecret, dbName)
}

// removedTables gives each removed table its reason: missing from the connection the human picked, or simply left out.
func removedTables(removed []string, dbName string, have []string) []vault.RemovedTable {
	var out []vault.RemovedTable
	missing := map[string]bool{}
	if dbName != "" {
		for _, t := range notDeclared(removed, have) {
			missing[t] = true
		}
	}
	for _, t := range removed {
		reason := "removed by the user"
		if missing[t] {
			reason = "does not exist in connection " + dbName
		}
		out = append(out, vault.RemovedTable{Table: t, Reason: reason})
	}
	return out
}

// notDeclared lists the names in got that are not in allowed. Names compare without case, since MySQL on some platforms and Postgres unquoted identifiers fold case, and HTTP methods are case-insensitive.
func notDeclared(got, allowed []string) []string {
	present := make(map[string]bool, len(allowed))
	for _, t := range allowed {
		present[strings.ToLower(t)] = true
	}
	var extra []string
	for _, t := range got {
		if !present[strings.ToLower(t)] {
			extra = append(extra, t)
		}
	}
	return extra
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
			Type: p.Type, TabID: p.TabID, Cwd: p.Cwd, Path: p.Path, AccountEnv: p.AccountEnv, Reason: p.Reason,
			Tables: p.Tables, Operations: p.Operations, Message: p.Message, Methods: p.Methods, Command: p.Command, Rule: p.Rule,
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
			a, err := svc.ExecuteAsk(p.ID, time.Duration(p.TimeoutSeconds)*time.Second, p.Rows, p.Cols)
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
			return response{OK: true}, svc.DeclineAsk(p.RootSecret, p.ID, p.Note, p.DBName, p.RemovedTables)
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
	return c.ExecuteAskSized(rootSecret, id, timeout, 0, 0)
}

// ExecuteAskSized starts the command on a terminal of rows by cols; a size out of range starts it at the default.
func (c *Client) ExecuteAskSized(rootSecret, id string, timeout time.Duration, rows, cols int) (AskView, error) {
	var out askPayload
	err := c.call(opExecuteAsk, executeAskPayload{RootSecret: rootSecret, ID: id, TimeoutSeconds: int(timeout / time.Second), Rows: rows, Cols: cols}, &out)
	return out.Ask, err
}

func (c *Client) StopAsk(rootSecret, id string) error {
	return c.call(opStopAsk, askIDPayload{RootSecret: rootSecret, ID: id}, nil)
}

func (c *Client) DeclineAsk(rootSecret, id, note string) error {
	return c.call(opDeclineAsk, declineAskPayload{RootSecret: rootSecret, ID: id, Note: note}, nil)
}

func (c *Client) DeclineAskRemoving(rootSecret, id, note, dbName string, removed []string) error {
	return c.call(opDeclineAsk, declineAskPayload{RootSecret: rootSecret, ID: id, Note: note, DBName: dbName, RemovedTables: removed}, nil)
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
