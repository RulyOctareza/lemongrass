// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/web"
)

const opWebCommand = "web_command"

// appSocketPath is a variable so tests can point the agent at a fake app.
var appSocketPath = web.SocketPath

type webCommandPayload struct {
	ShortID string            `json:"short_id"`
	TabID   string            `json:"tab_id"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Flags   map[string]string `json:"flags"`
}

type webResultPayload struct {
	Result web.Result `json:"result"`
}

// WebCommand resolves shortID, checks through the vault that it is an active web channel, and hands the command to the app's browser engine with the real channel id, which the engine needs to ask the vault for login fields. An api channel is refused naming the commands it does take.
func (s *Service) WebCommand(shortID, tabID, command string, args []string, flags map[string]string) (web.Result, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return web.Result{}, ErrNoSuchChannel
	}
	if tabID == "" {
		return web.Result{}, errors.New("agent: web commands only work inside a lemongrass tab, LGRASS_TAB_ID is not set")
	}
	info, err := s.vaultClient.HTTPChannelInfo(realID)
	if err != nil {
		return web.Result{}, redactID(err, realID, shortID)
	}
	if info.Type == "app" {
		return web.Result{}, fmt.Errorf("agent: this is an app channel, it takes app-login, info and users, not %s", command)
	}
	if info.Type != "web" {
		return web.Result{}, fmt.Errorf("agent: this is an api channel, it takes get, post, put, patch, delete, head, options and flush, not %s", command)
	}
	if info.Status != "active" {
		return web.Result{}, fmt.Errorf("agent: this channel is %s", info.Status)
	}
	res, err := web.Call(appSocketPath(), web.Request{TabID: tabID, ChannelID: string(realID), Command: command, Args: args, Flags: flags})
	return res, redactID(err, realID, shortID)
}

func webCommandOp(svc *Service, queryLimiter *vault.FailureLimiter, payload json.RawMessage) response {
	var p webCommandPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errResponse(err)
	}
	return queryOp(queryLimiter, func() (response, error) {
		result, err := svc.WebCommand(p.ShortID, p.TabID, p.Command, p.Args, p.Flags)
		return payloadResponse(webResultPayload{Result: result}), err
	})
}

func (c *Client) WebCommand(shortID, tabID, command string, args []string, flags map[string]string) (web.Result, error) {
	var out webResultPayload
	err := c.callWithin(max(c.Timeout, web.Timeout), opWebCommand, webCommandPayload{ShortID: shortID, TabID: tabID, Command: command, Args: args, Flags: flags}, &out)
	return out.Result, err
}
