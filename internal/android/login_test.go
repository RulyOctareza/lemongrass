// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package android

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const secret = "s3cr&t pa$$"

// fakeDevice shows nodes after a number of dumps, records every call, and fails the way a
// careless device would, with the typed text in the error.
type fakeDevice struct {
	nodes      []Node
	readyAfter int
	dumps      int
	failText   bool
	calls      []string
}

func (f *fakeDevice) Launch(_ context.Context, pkg string) error {
	f.calls = append(f.calls, "launch "+pkg)
	return nil
}

func (f *fakeDevice) Dump(context.Context) ([]Node, error) {
	f.dumps++
	if f.dumps <= f.readyAfter {
		return nil, nil
	}
	return f.nodes, nil
}

func (f *fakeDevice) Tap(_ context.Context, x, y int) error {
	f.calls = append(f.calls, fmt.Sprintf("tap %d,%d", x, y))
	return nil
}

func (f *fakeDevice) Text(_ context.Context, text string) error {
	if f.failText {
		return fmt.Errorf("input text %s: broken", text)
	}
	f.calls = append(f.calls, "text "+text)
	return nil
}

func (f *fakeDevice) ClearField(_ context.Context, chars int) error {
	f.calls = append(f.calls, fmt.Sprintf("clear %d", chars))
	return nil
}

func (f *fakeDevice) Key(_ context.Context, key string) error {
	f.calls = append(f.calls, "key "+key)
	return nil
}

func fastPolling(t *testing.T) {
	t.Helper()
	poll, settle := pollInterval, fillSettle
	pollInterval, fillSettle = time.Millisecond, 0
	t.Cleanup(func() { pollInterval, fillSettle = poll, settle })
}

func loginScreen() []Node {
	return []Node{
		{ID: "com.app:id/login_email", Text: "mandor", Clickable: true, Bounds: [4]int{0, 100, 100, 200}},
		{Desc: "Password", Clickable: true, Bounds: [4]int{0, 300, 100, 400}},
		{Desc: "Sign in", Clickable: true, Bounds: [4]int{0, 500, 100, 600}},
	}
}

var loginSteps = []AppStep{
	{Action: ActionLaunch},
	{Action: ActionWait, ID: "login_email"},
	{Action: ActionFill, ID: "login_email", Field: "username"},
	{Action: ActionFill, Text: "Password", Field: "password"},
	{Action: ActionTap, Text: "Sign in"},
	{Action: ActionKey, Key: "back"},
}

func fields(m map[string]string) FieldLookup {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestRunAppStepsRunsInOrderAndWaits(t *testing.T) {
	fastPolling(t)
	dev := &fakeDevice{nodes: loginScreen(), readyAfter: 3}
	err := runAppSteps(context.Background(), dev, "com.app", loginSteps,
		fields(map[string]string{"username": "ana", "password": secret}))
	if err != nil {
		t.Fatalf("runAppSteps: %v", err)
	}
	// The prefilled email is cleared by its length plus the margin, the empty password by the margin.
	want := []string{"launch com.app", "tap 50,150", "clear 70", "text ana", "tap 50,350", "clear 64", "text " + secret, "tap 50,550", "key back"}
	if strings.Join(dev.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %q", dev.calls)
	}
}

func TestRunAppStepsChecksEveryFieldBeforeTyping(t *testing.T) {
	cases := map[string]map[string]string{
		"missing":   {"username": "ana"},
		"non-ascii": {"username": "ana", "password": "pässword"},
	}
	for name, values := range cases {
		dev := &fakeDevice{nodes: loginScreen()}
		err := runAppSteps(context.Background(), dev, "com.app", loginSteps, fields(values))
		var se *StepError
		if !errors.As(err, &se) || se.Step != 4 || se.Action != ActionFill {
			t.Fatalf("%s: err = %v", name, err)
		}
		if !strings.Contains(err.Error(), `"password"`) || strings.Contains(err.Error(), "ä") {
			t.Errorf("%s: err = %v", name, err)
		}
		if len(dev.calls) != 0 {
			t.Errorf("%s: ran %q before refusing", name, dev.calls)
		}
	}
}

func TestRunAppStepsTimeoutNamesTheStep(t *testing.T) {
	fastPolling(t)
	steps := []AppStep{{Action: ActionLaunch}, {Action: ActionTap, Text: "Checkout", TimeoutSeconds: 1}}
	err := runAppSteps(context.Background(), &fakeDevice{nodes: loginScreen()}, "com.app", steps, fields(nil))
	if err == nil || !strings.HasPrefix(err.Error(), "step 2 (tap): not found within 1s") || !strings.Contains(err.Error(), `"Checkout"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAppStepsNeverLeaksTheValue(t *testing.T) {
	fastPolling(t)
	dev := &fakeDevice{nodes: loginScreen(), failText: true}
	err := runAppSteps(context.Background(), dev, "com.app", loginSteps,
		fields(map[string]string{"username": secret, "password": secret}))
	if err == nil || !strings.HasPrefix(err.Error(), "step 3 (fill)") || strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", loginSteps), secret) {
		t.Fatal("a step holds the value")
	}
}

func TestRunAppStepsRefusesBadSteps(t *testing.T) {
	cases := []struct {
		pkg  string
		step AppStep
	}{
		{"", AppStep{Action: ActionLaunch}},
		{"com.app", AppStep{Action: ActionTap}},
		{"com.app", AppStep{Action: ActionFill, ID: "x"}},
		{"com.app", AppStep{Action: ActionKey}},
		{"com.app", AppStep{Action: "swipe"}},
		{"com.app", AppStep{Action: ActionWait, TimeoutSeconds: -1}},
	}
	for _, c := range cases {
		dev := &fakeDevice{}
		err := runAppSteps(context.Background(), dev, c.pkg, []AppStep{{Action: ActionKey, Key: "back"}, c.step}, fields(nil))
		var se *StepError
		if !errors.As(err, &se) || se.Step != 2 || len(dev.calls) != 0 {
			t.Errorf("%+v: err = %v, calls %q", c.step, err, dev.calls)
		}
	}
}

func TestRunAppStepsStopsOnCancel(t *testing.T) {
	fastPolling(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runAppSteps(ctx, &fakeDevice{}, "com.app", []AppStep{{Action: ActionWait, Text: "never", TimeoutSeconds: 60}}, fields(nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestFillClearRunIsBounded(t *testing.T) {
	fastPolling(t)
	dev := &fakeDevice{nodes: []Node{{ID: "notes", Text: strings.Repeat("x", 500), Bounds: [4]int{0, 0, 10, 10}}}}
	steps := []AppStep{{Action: ActionFill, ID: "notes", Field: "note"}}
	if err := runAppSteps(context.Background(), dev, "com.app", steps, fields(map[string]string{"note": "hi"})); err != nil {
		t.Fatalf("runAppSteps: %v", err)
	}
	if dev.calls[1] != "clear 256" {
		t.Fatalf("calls = %q", dev.calls)
	}
}
