// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/android"
	"github.com/faizalv/lemongrass/internal/vault"
)

const appPassword = "s3cret&pw;$x"

// fakePhone stands in for android.Client: it records what the gate hands the executor and returns its canned error.
type fakePhone struct {
	pkg    string
	steps  []android.AppStep
	values map[string]string
	err    error
}

func (f *fakePhone) RunAppSteps(_ context.Context, pkg string, steps []android.AppStep, field android.FieldLookup) error {
	f.pkg, f.steps, f.values = pkg, steps, map[string]string{}
	for _, s := range steps {
		if s.Action == android.ActionFill {
			f.values[s.Field], _ = field(s.Field)
		}
	}
	return f.err
}

func useFakePhone(t *testing.T, phone *fakePhone) *string {
	t.Helper()
	var serial string
	orig := openAppDevice
	openAppDevice = func(_ context.Context, s string) (appRunner, error) {
		serial = s
		return phone, nil
	}
	t.Cleanup(func() { openAppDevice = orig })
	return &serial
}

// setUpAppChannel stores a rester with an api and an app side and returns an app and an api channel on staging.
func setUpAppChannel(t *testing.T) (svc *testSvc, appID, apiID vault.ChannelID) {
	t.Helper()
	svc = newTestService(t)
	const root = "root-secret"
	if err := svc.SetPassphrase(root); err != nil {
		t.Fatal(err)
	}
	r := vault.Rester{
		API: &vault.ResterAPI{LoginEndpoint: "/login", TokenPath: "access_token"},
		App: &vault.ResterApp{Package: "com.example.shop", Login: []vault.AppStep{
			{Action: vault.AppStepLaunch},
			{Action: vault.AppStepFill, ID: "email", Field: "username"},
			{Action: vault.AppStepFill, Text: "Password", Field: "password"},
			{Action: vault.AppStepTap, Text: "Sign in", TimeoutSeconds: 5},
		}},
		Environments: []vault.Environment{{Name: "staging", BaseURL: "https://api.staging.test"}},
		Users: []vault.ResterUser{
			{Name: "Alice Doe", Fields: map[string]string{"username": "alice", "password": "x"}, Tags: []string{"admin"}, Environments: []string{"staging"},
				Overrides: map[string]vault.UserOverride{"staging": {Fields: map[string]string{"password": appPassword}}}},
			{Name: "bob", Fields: map[string]string{"username": "bob", "password": "bob-pw"}, Environments: []string{"staging"}},
			{Name: "bot", Token: "s3cret-token", Environments: []string{"staging"}},
		},
	}
	if err := svc.PutRester(root, "shop", r); err != nil {
		t.Fatal(err)
	}
	app, err := svc.CreateHTTPChannel(root, "phone", "shop", "staging", vault.TypeApp, vault.HTTPScope{}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	api, err := svc.CreateHTTPChannel(root, "api", "shop", "staging", vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, app.ID, api.ID
}

func TestAppLoginHandsTheExecutorTheEffectiveUsersFields(t *testing.T) {
	svc, appID, _ := setUpAppChannel(t)
	phone := &fakePhone{}
	serial := useFakePhone(t, phone)

	res, err := svc.AppLogin(appID, "Alice_Doe", "R58M")
	if err != nil {
		t.Fatalf("AppLogin: %v", err)
	}
	if !res.OK || res.User != "alice_doe" || res.FailedStep != 0 {
		t.Errorf("result = %+v, want ok as alice_doe", res)
	}
	if *serial != "R58M" || phone.pkg != "com.example.shop" || len(phone.steps) != 4 || phone.steps[3].TimeoutSeconds != 5 || phone.steps[1].ID != "email" {
		t.Errorf("executor got serial %q pkg %q steps %+v", *serial, phone.pkg, phone.steps)
	}
	if phone.values["username"] != "alice" || phone.values["password"] != appPassword {
		t.Errorf("fill values = %v, want the staging override applied", phone.values)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), appPassword) || strings.Contains(string(raw), "alice\"") {
		t.Errorf("result carries a field value: %s", raw)
	}
}

func TestAppLoginUserResolution(t *testing.T) {
	svc, appID, _ := setUpAppChannel(t)
	useFakePhone(t, &fakePhone{})

	if _, err := svc.AppLogin(appID, "", ""); err == nil || !strings.Contains(err.Error(), "alice_doe") || !strings.Contains(err.Error(), "bob") || strings.Contains(err.Error(), "bot") {
		t.Errorf("empty user with several users = %v, want the login handles listed", err)
	}
	if _, err := svc.AppLogin(appID, "bot", ""); err == nil {
		t.Error("a token user logged in on an app channel")
	}
	if _, err := svc.AppLogin(appID, "nobody", ""); err == nil {
		t.Error("an unknown user logged in")
	}
}

func TestAppLoginFailedStepIsAResultWithoutTheSecret(t *testing.T) {
	svc, appID, _ := setUpAppChannel(t)
	phone := &fakePhone{err: &android.StepError{Step: 3, Action: "fill", Err: errors.New("typing " + appPassword + " failed")}}
	useFakePhone(t, phone)

	res, err := svc.AppLogin(appID, "bob", "")
	if err != nil {
		t.Fatalf("AppLogin: %v, want a failed step to be a result", err)
	}
	if res.OK || res.User != "bob" || res.FailedStep != 3 || res.FailedAction != "fill" || res.Reason == "" {
		t.Errorf("result = %+v, want failure at step 3 (fill) as bob", res)
	}

	phone.err = &android.StepError{Step: 2, Action: "fill", Err: errors.New("field \"password\": bob-pw is not typeable")}
	res, _ = svc.AppLogin(appID, "bob", "")
	if strings.Contains(res.Reason, "bob-pw") || !strings.Contains(res.Reason, "[redacted]") {
		t.Errorf("reason = %q, want the field value scrubbed", res.Reason)
	}
}

func TestAppLoginDeviceErrorCarriesNoSecret(t *testing.T) {
	svc, appID, _ := setUpAppChannel(t)
	orig := openAppDevice
	openAppDevice = func(context.Context, string) (appRunner, error) {
		return nil, errors.New("android: no Android device is connected")
	}
	defer func() { openAppDevice = orig }()

	if _, err := svc.AppLogin(appID, "bob", ""); err == nil || !strings.Contains(err.Error(), "no Android device") {
		t.Errorf("AppLogin with no phone = %v, want the device error", err)
	}

	useFakePhone(t, &fakePhone{err: errors.New("adb died with bob-pw")})
	_, err := svc.AppLogin(appID, "bob", "")
	if err == nil || strings.Contains(err.Error(), "bob-pw") {
		t.Errorf("non-step error = %v, want an error without the value", err)
	}
}

func TestAppAndApiChannelsRefuseEachOthersCommands(t *testing.T) {
	svc, appID, apiID := setUpAppChannel(t)
	useFakePhone(t, &fakePhone{})

	if _, err := svc.AppLogin(apiID, "alice_doe", ""); err == nil || !strings.HasPrefix(err.Error(), "restergate: ") || !strings.Contains(err.Error(), "api channel") {
		t.Errorf("AppLogin on an api channel = %v, want a refusal naming the type", err)
	}
	if _, err := svc.RequestHTTP(appID, "alice_doe", "GET", "/x", nil, "", nil); err == nil || !strings.Contains(err.Error(), "app channel") || !strings.Contains(err.Error(), "app-login") {
		t.Errorf("RequestHTTP on an app channel = %v, want a refusal naming app-login", err)
	}
	if _, err := svc.FlushHTTPTokens(appID, ""); err == nil || !strings.Contains(err.Error(), "app channel") {
		t.Errorf("FlushHTTPTokens on an app channel = %v, want a refusal", err)
	}
	if _, err := svc.DownloadHTTP(appID, "alice_doe", "GET", "/x", nil, "", nil); err == nil || !strings.Contains(err.Error(), "app channel") {
		t.Errorf("DownloadHTTP on an app channel = %v, want a refusal", err)
	}
	if _, err := svc.WebLoginFields(appID, "alice_doe"); err == nil {
		t.Error("an app channel gave out web login fields")
	}
}

func TestAppInfoShowsPackageAndNoSecret(t *testing.T) {
	svc, appID, _ := setUpAppChannel(t)

	info, err := svc.HTTPChannelInfo(appID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != vault.TypeApp || info.Package != "com.example.shop" || info.BaseURL != "" || info.Origin != "" || len(info.Users) != 2 {
		t.Errorf("info = %+v, want the package and the two login users", info)
	}
	raw, _ := json.Marshal(info)
	for _, secret := range []string{appPassword, "bob-pw", "s3cret-token"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("info leaks %q: %s", secret, raw)
		}
	}
}
