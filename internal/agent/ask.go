// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"encoding/json"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
)

// The agent relays the model's ask ops to the vault unchanged: the vault accepts them only from the lemongrass daemon binary, which the agent is and the lgrass CLI is not. The app's ask ops are not relayed, so nothing reachable from lgrass can answer an ask.
const (
	opCreateAsk = "create_ask"
	opWaitAsk   = "wait_ask"
	opAskResult = "ask_result"
	opAckAsk    = "ack_ask"
)

// waitAskSlack covers the relay's own hop on top of the vault's wait.
const waitAskSlack = time.Minute

type askRefPayload struct {
	ID             string `json:"id"`
	TabID          string `json:"tab_id"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type askReplyPayload struct {
	Ask vault.Ask `json:"ask"`
}

func dispatchAsk(svc *Service, req request) (response, bool) {
	switch req.Op {
	case opCreateAsk:
		var p gatekeeper.AskRequest
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		a, err := svc.vaultClient.CreateAsk(p)
		if err != nil {
			return errResponse(err), true
		}
		return payloadResponse(askReplyPayload{Ask: a}), true

	case opWaitAsk:
		var p askRefPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		a, err := svc.vaultClient.WaitAsk(p.ID, p.TabID, askWait(p.TimeoutSeconds))
		if err != nil {
			return errResponse(err), true
		}
		return payloadResponse(askReplyPayload{Ask: a}), true

	case opAskResult:
		var p askRefPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		a, err := svc.vaultClient.AskResult(p.ID, p.TabID)
		if err != nil {
			return errResponse(err), true
		}
		return payloadResponse(askReplyPayload{Ask: a}), true

	case opAckAsk:
		var p askRefPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err), true
		}
		if err := svc.vaultClient.AckAsk(p.ID, p.TabID); err != nil {
			return errResponse(err), true
		}
		return response{OK: true}, true
	}
	return response{}, false
}

func askWait(seconds int) time.Duration {
	wait := time.Duration(seconds) * time.Second
	if wait <= 0 || wait > vault.AskMaxWait {
		return vault.AskMaxWait
	}
	return wait
}

// CreateAsk files an ask through the agent.
func (c *Client) CreateAsk(req gatekeeper.AskRequest) (vault.Ask, error) {
	var out askReplyPayload
	err := c.call(opCreateAsk, req, &out)
	return out.Ask, err
}

// WaitAsk blocks until the ask has its outcome or wait passes.
func (c *Client) WaitAsk(id, tabID string, wait time.Duration) (vault.Ask, error) {
	var out askReplyPayload
	err := c.callWithin(wait+waitAskSlack, opWaitAsk, askRefPayload{ID: id, TabID: tabID, TimeoutSeconds: int(wait / time.Second)}, &out)
	return out.Ask, err
}

// AskResult reads an ask this tab filed.
func (c *Client) AskResult(id, tabID string) (vault.Ask, error) {
	var out askReplyPayload
	err := c.call(opAskResult, askRefPayload{ID: id, TabID: tabID}, &out)
	return out.Ask, err
}

// AckAsk confirms the model has the ask's outcome.
func (c *Client) AckAsk(id, tabID string) error {
	return c.call(opAckAsk, askRefPayload{ID: id, TabID: tabID}, nil)
}
