// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package android

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// App login step actions, the same words as the vault's app steps.
const (
	ActionLaunch = "launch"
	ActionTap    = "tap"
	ActionFill   = "fill"
	ActionKey    = "key"
	ActionWait   = "wait"
)

// DefaultStepTimeout bounds a step whose TimeoutSeconds is 0.
const DefaultStepTimeout = 10 * time.Second

// AppStep is one step of an app login. tap and fill find their element by Text or ID, waiting
// for it to appear; fill then clears the field and types the value of the user field named by Field; key presses Key;
// wait waits for Text or ID, or pauses for the timeout when it names neither. A step never holds
// a value, so printing one cannot leak a credential.
type AppStep struct {
	Action         string
	Text           string
	ID             string
	Field          string
	Key            string
	TimeoutSeconds int
}

// FieldLookup returns the value of a user field, already resolved for the environment.
type FieldLookup func(field string) (string, bool)

// StepError is a failed login step. Its message names the step, its target and its field key,
// never a typed value.
type StepError struct {
	Step   int
	Action string
	Err    error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("step %d (%s): %v", e.Step, e.Action, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }

// appDevice is what the login runner needs from a phone, so tests can run it without one.
type appDevice interface {
	Launch(ctx context.Context, pkg string) error
	Dump(ctx context.Context) ([]Node, error)
	Tap(ctx context.Context, x, y int) error
	Text(ctx context.Context, text string) error
	ClearField(ctx context.Context, chars int) error
	Key(ctx context.Context, key string) error
}

// pollInterval is the pause between dumps while a step waits for its element, and fillSettle
// the pause between tapping a field and typing, so the field has focus before keys arrive.
var (
	pollInterval = 500 * time.Millisecond
	fillSettle   = 300 * time.Millisecond
)

// clearMargin is how many deletes fill sends beyond the length the dump shows, since a field's
// text can be hidden from the dump (a password, or a label node drawn over the field), and
// clearLimit bounds the run.
const (
	clearMargin = 64
	clearLimit  = 256
)

// RunAppSteps logs an app in by running steps in order. Every fill field is resolved and checked
// before the first step, so a missing or untypeable value fails with nothing typed.
func (c *Client) RunAppSteps(ctx context.Context, pkg string, steps []AppStep, field FieldLookup) error {
	return runAppSteps(ctx, c, pkg, steps, field)
}

func runAppSteps(ctx context.Context, dev appDevice, pkg string, steps []AppStep, field FieldLookup) error {
	values := make([]string, len(steps))
	for i, s := range steps {
		if err := checkStep(s, pkg); err != nil {
			return &StepError{Step: i + 1, Action: s.Action, Err: err}
		}
		if s.Action != ActionFill {
			continue
		}
		value, ok := field(s.Field)
		if !ok {
			return &StepError{Step: i + 1, Action: s.Action, Err: fmt.Errorf("the user has no field %q", s.Field)}
		}
		if err := CheckTypeable(value); err != nil {
			return &StepError{Step: i + 1, Action: s.Action, Err: fmt.Errorf("field %q: %w", s.Field, err)}
		}
		values[i] = value
	}
	for i, s := range steps {
		if err := runStep(ctx, dev, pkg, s, values[i]); err != nil {
			return &StepError{Step: i + 1, Action: s.Action, Err: err}
		}
	}
	return nil
}

func checkStep(s AppStep, pkg string) error {
	switch s.Action {
	case ActionLaunch:
		if pkg == "" {
			return errors.New("no package to launch")
		}
	case ActionTap, ActionFill:
		if s.Text == "" && s.ID == "" {
			return errors.New("needs a text or an id to find")
		}
		if s.Action == ActionFill && s.Field == "" {
			return errors.New("needs a field")
		}
	case ActionKey:
		if s.Key == "" {
			return errors.New("needs a key")
		}
	case ActionWait:
	default:
		return errors.New("unknown action, want launch, tap, fill, key or wait")
	}
	if s.TimeoutSeconds < 0 {
		return errors.New("negative timeout")
	}
	return nil
}

func runStep(ctx context.Context, dev appDevice, pkg string, s AppStep, value string) error {
	timeout := DefaultStepTimeout
	if s.TimeoutSeconds > 0 {
		timeout = time.Duration(s.TimeoutSeconds) * time.Second
	}
	switch s.Action {
	case ActionLaunch:
		return dev.Launch(ctx, pkg)
	case ActionKey:
		return dev.Key(ctx, s.Key)
	case ActionWait:
		if s.Text == "" && s.ID == "" {
			return pause(ctx, timeout)
		}
		_, err := waitFor(ctx, dev, s, timeout)
		return err
	}
	n, err := waitFor(ctx, dev, s, timeout)
	if err != nil {
		return err
	}
	x, y := n.Center()
	if err := dev.Tap(ctx, x, y); err != nil {
		return err
	}
	if s.Action == ActionTap {
		return nil
	}
	if err := pause(ctx, fillSettle); err != nil {
		return err
	}
	// An app may prefill a field, such as a debug build's username, and typing would append to it.
	if err := dev.ClearField(ctx, min(utf8.RuneCountInString(n.Text)+clearMargin, clearLimit)); err != nil {
		return fmt.Errorf("clearing the field: %w", err)
	}
	if err := dev.Text(ctx, value); err != nil {
		// Text keeps the value out of its errors; this guards against a device that does not.
		if value != "" && strings.Contains(err.Error(), value) {
			return fmt.Errorf("typing field %q failed", s.Field)
		}
		return fmt.Errorf("typing field %q: %w", s.Field, err)
	}
	return nil
}

// waitFor dumps the screen until the step's element appears or the timeout passes. A dump can
// fail while the screen changes, so only the last failure is reported.
func waitFor(ctx context.Context, dev appDevice, s AppStep, timeout time.Duration) (Node, error) {
	deadline := time.Now().Add(timeout)
	for {
		n, err := find(ctx, dev, s)
		if err == nil {
			return n, nil
		}
		if ctx.Err() != nil {
			return Node{}, ctx.Err()
		}
		if !time.Now().Add(pollInterval).Before(deadline) {
			return Node{}, fmt.Errorf("not found within %s: %w", timeout, err)
		}
		if err := pause(ctx, pollInterval); err != nil {
			return Node{}, err
		}
	}
}

func find(ctx context.Context, dev appDevice, s AppStep) (Node, error) {
	nodes, err := dev.Dump(ctx)
	if err != nil {
		return Node{}, err
	}
	if s.ID != "" {
		return FindByID(nodes, s.ID)
	}
	return FindByText(nodes, s.Text)
}

func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
