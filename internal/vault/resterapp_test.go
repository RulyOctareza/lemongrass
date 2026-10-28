// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"strings"
	"testing"
	"time"
)

func appRester() Rester {
	return Rester{
		App: &ResterApp{Package: "com.example.shop", Login: []AppStep{
			{Action: AppStepLaunch},
			{Action: AppStepTap, Text: "Sign in"},
			{Action: AppStepFill, ID: "email", Field: "username"},
			{Action: AppStepFill, Text: "Password", Field: "password"},
			{Action: AppStepKey, Key: "KEYCODE_ENTER"},
			{Action: AppStepWait, Text: "Home", TimeoutSeconds: 20},
		}},
		Environments: []Environment{{Name: "staging"}, {Name: "prod"}},
		Users: []ResterUser{
			{Name: "alice", Fields: map[string]string{"username": "alice", "password": "s3cret&x"}, Environments: []string{"staging", "prod"},
				Overrides: map[string]UserOverride{"prod": {Fields: map[string]string{"password": "prod-pw"}}}},
			{Name: "nopass", Fields: map[string]string{"username": "n"}, Environments: []string{"staging"}},
			{Name: "bot", Token: "t", Environments: []string{"staging"}},
		},
	}
}

func TestValidateAppSide(t *testing.T) {
	if err := appRester().Validate(); err != nil {
		t.Fatalf("Validate = %v, want nil for an app-only rester with no address", err)
	}
	cases := []struct {
		name   string
		mutate func(*Rester)
		want   string
	}{
		{"no package", func(r *Rester) { r.App.Package = " " }, "package name"},
		{"unknown action", func(r *Rester) { r.App.Login[0].Action = "swipe" }, "unknown action"},
		{"tap without locator", func(r *Rester) { r.App.Login[1] = AppStep{Action: AppStepTap} }, "step 2 (tap) needs a text or an id"},
		{"fill without field", func(r *Rester) { r.App.Login[2].Field = "" }, "step 3 (fill) needs"},
		{"key without key", func(r *Rester) { r.App.Login[4].Key = "" }, "step 5 (key) needs a key"},
		{"wait without locator", func(r *Rester) { r.App.Login[5] = AppStep{Action: AppStepWait} }, "step 6 (wait)"},
		{"negative timeout", func(r *Rester) { r.App.Login[5].TimeoutSeconds = -1 }, "negative timeout"},
	}
	for _, c := range cases {
		r := appRester()
		c.mutate(&r)
		err := r.Validate()
		if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Validate = %v, want a vault error containing %q", c.name, err, c.want)
		}
	}
	if err := (Rester{Environments: []Environment{{Name: "x"}}}).Validate(); err == nil {
		t.Error("a rester with no side validated")
	}
}

func TestCanUseApp(t *testing.T) {
	r := appRester()
	alice, nopass, bot := r.Users[0], r.Users[1], r.Users[2]
	cases := []struct {
		name string
		env  string
		u    ResterUser
		want bool
	}{
		{"login user with every field", "staging", alice, true},
		{"environment needs no address", "prod", alice, true},
		{"user missing a fill field", "staging", nopass, false},
		{"token user never on app", "staging", bot, false},
		{"unassigned environment", "prod", nopass, false},
	}
	for _, c := range cases {
		if got := r.CanUse(TypeApp, c.env, c.u); got != c.want {
			t.Errorf("%s: CanUse = %v, want %v", c.name, got, c.want)
		}
	}
	noApp := multiEnvRester()
	if noApp.CanUse(TypeApp, "local", noApp.Users[0]) {
		t.Error("a rester with no app side was usable as an app channel")
	}
}

func TestResolveAppCarriesPackageStepsAndEffectiveUsers(t *testing.T) {
	r := appRester()
	got, err := r.resolve("prod", TypeApp)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Package != "com.example.shop" || len(got.AppLogin) != len(r.App.Login) || got.Origin != "" || got.Login != nil {
		t.Errorf("resolved = %+v, want the package and app steps only", got)
	}
	if len(got.Users) != 1 || got.Users[0].Fields["password"] != "prod-pw" {
		t.Errorf("users = %+v, want alice with the prod override applied", got.Users)
	}
	got.AppLogin[0].Action = "changed"
	if r.App.Login[0].Action != AppStepLaunch {
		t.Error("resolve shared the step slice with the rester")
	}
	if _, err := multiEnvRester().resolve("local", TypeApp); err == nil || !strings.Contains(err.Error(), "no app side") {
		t.Errorf("resolve without an app side = %v, want an error", err)
	}
}

func TestAppChannelRoundTrip(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutRester(testRootSecret, "shop", appRester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	got, err := svc.GetRester(testRootSecret, "shop")
	if err != nil || got.App == nil || got.App.Package != "com.example.shop" || len(got.App.Login) != 6 || got.App.Login[5].TimeoutSeconds != 20 {
		t.Fatalf("GetRester = %+v, %v, want the app side back", got.App, err)
	}
	c, err := svc.CreateHTTPChannel(testRootSecret, "phone", "shop", "staging", TypeApp, HTTPScope{Methods: []string{"GET"}}, &WebScope{}, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}
	if c.Type != TypeApp || c.WebScope != nil || len(c.Scope.Methods) != 0 {
		t.Errorf("channel = %+v, want an app channel with no api or web scope", c)
	}
	_, resolved, err := svc.OpenHTTPChannel(c.ID)
	if err != nil || resolved.Package != "com.example.shop" || len(resolved.AppLogin) != 6 || len(resolved.Users) != 1 {
		t.Errorf("OpenHTTPChannel = %+v, %v, want the app side and one user", resolved, err)
	}
	if _, _, err := svc.OpenWebChannel(c.ID); err == nil {
		t.Error("OpenWebChannel on an app channel = nil, want a refusal")
	}
	if _, err := svc.CreateHTTPChannel(testRootSecret, "x", "shop", "prod", TypeAPI, HTTPScope{}, nil, time.Hour); err == nil {
		t.Error("an api channel on an app-only rester was created")
	}
}
