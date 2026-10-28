// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/android"
	"github.com/faizalv/lemongrass/internal/vault"
)

// AppLoginTimeout bounds one app-login end to end at every hop, since each step may wait for a screen.
var AppLoginTimeout = 120 * time.Second

// minScrubLen is the shortest field value scrubbed from an error. A shorter one would also match
// ordinary words and digits, and the executor already keeps values out of its errors.
const minScrubLen = 3

// appRunner is the part of android.Client a login needs, so tests can run the gate without a phone.
type appRunner interface {
	RunAppSteps(ctx context.Context, pkg string, steps []android.AppStep, field android.FieldLookup) error
}

// openAppDevice is a variable so tests can stand in for a phone.
var openAppDevice = func(ctx context.Context, serial string) (appRunner, error) {
	return android.Open(ctx, serial)
}

// AppLoginResult is what a model reads after an app-login: the user it acted as and either ok or
// the step that failed, with a reason that never holds a field value.
type AppLoginResult struct {
	User         string `json:"user"`
	OK           bool   `json:"ok"`
	FailedStep   int    `json:"failed_step,omitempty"`
	FailedAction string `json:"failed_action,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// openAppChannel opens an active channel for an app operation, refusing an api or a web channel.
func (g *Gate) openAppChannel(id vault.ChannelID) (vault.ResolvedRester, error) {
	c, rester, err := g.vault.OpenHTTPChannel(id)
	if err != nil {
		return vault.ResolvedRester{}, err
	}
	if c.Type != vault.TypeApp {
		return vault.ResolvedRester{}, fmt.Errorf("restergate: this is a %s channel for the %s environment, so it does not take app-login, only an app channel does", c.Type, c.Environment)
	}
	return rester, nil
}

// AppLogin logs the channel's app in on the connected phone as user (a handle, or empty for a
// channel with one user). The user's field values are resolved here, with the environment's
// overrides already applied, and go only to the executor; the result and every error leave them
// out. A failed step is a result, not an error, so the caller still learns which user acted.
// serial picks a phone and may be empty when one is connected.
func (g *Gate) AppLogin(id vault.ChannelID, user, serial string) (AppLoginResult, error) {
	rester, err := g.openAppChannel(id)
	if err != nil {
		return AppLoginResult{}, err
	}
	name, err := resolveUser(rester, user)
	if err != nil {
		return AppLoginResult{}, err
	}
	u, err := findResterUser(rester, name)
	if err != nil {
		return AppLoginResult{}, err
	}
	res := AppLoginResult{User: vault.UserHandle(u.Name)}

	steps := make([]android.AppStep, len(rester.AppLogin))
	for i, s := range rester.AppLogin {
		steps[i] = android.AppStep{Action: s.Action, Text: s.Text, ID: s.ID, Field: s.Field, Key: s.Key, TimeoutSeconds: s.TimeoutSeconds}
	}
	lookup := func(field string) (string, bool) {
		v, ok := u.Fields[field]
		return v, ok
	}

	ctx, cancel := context.WithTimeout(context.Background(), AppLoginTimeout)
	defer cancel()
	dev, err := openAppDevice(ctx, serial)
	if err != nil {
		return AppLoginResult{}, scrubError(err, u)
	}
	err = dev.RunAppSteps(ctx, rester.Package, steps, lookup)
	if err == nil {
		res.OK = true
		return res, nil
	}
	var se *android.StepError
	if !errors.As(err, &se) {
		return AppLoginResult{}, scrubError(err, u)
	}
	res.FailedStep, res.FailedAction = se.Step, se.Action
	res.Reason = scrubError(se.Err, u).Error()
	return res, nil
}

// scrubError returns err with every field value of u cut out, as a last guard if the executor ever lets one into a message.
func scrubError(err error, u vault.ResterUser) error {
	msg := err.Error()
	for _, v := range u.Fields {
		if len(v) >= minScrubLen {
			msg = strings.ReplaceAll(msg, v, "[redacted]")
		}
	}
	return errors.New(msg)
}
