// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"strings"
	"testing"
	"time"
)

// multiEnvRester has local, staging and prod, an api and a web side, and users shaped to exercise assignment and overrides.
func multiEnvRester() Rester {
	return Rester{
		API: &ResterAPI{
			LoginEndpoint:  "/login",
			TokenPath:      "token",
			TokenPlacement: TokenPlacement{Kind: PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
			Headers:        []Header{{Name: "X-App", Value: "shared"}, {Name: "X-Tenant", Value: "shared"}},
		},
		Web: &ResterWeb{Login: []LoginStep{
			{Action: StepGoto, URL: "/login"},
			{Action: StepFill, Selector: "#u", Field: "username"},
			{Action: StepFill, Selector: "#p", Field: "password"},
			{Action: StepClick, Selector: "button"},
		}},
		Environments: []Environment{
			{Name: "local", BaseURL: "http://localhost:8080", Origin: "http://localhost:3000"},
			{Name: "staging", BaseURL: "https://api.staging.test", Origin: "https://staging.test", Headers: []Header{{Name: "x-tenant", Value: "staging"}}},
			{Name: "prod", BaseURL: "https://api.test"},
		},
		Users: []ResterUser{
			{
				Name:         "alice",
				Fields:       map[string]string{"username": "alice", "password": "pw", "site": "1"},
				FieldTypes:   map[string]string{"site": FieldNumber},
				Tags:         []string{"admin"},
				Environments: []string{"local", "staging", "prod"},
				Overrides: map[string]UserOverride{
					"staging": {Fields: map[string]string{"password": "staging-pw", "site": "2"}},
				},
			},
			{Name: "bot", Token: "pasted", Environments: []string{"local", "prod"}, Overrides: map[string]UserOverride{"prod": {Token: "prod-token"}}},
			{Name: "idle", Fields: map[string]string{"username": "idle"}},
			{Name: "apionly", Fields: map[string]string{"username": "x"}, Environments: []string{"local"}},
		},
	}
}

func TestEffectiveUserAppliesOverridesKeyByKey(t *testing.T) {
	alice := multiEnvRester().Users[0]

	local := alice.Effective("local")
	if local.Fields["password"] != "pw" || local.Fields["site"] != "1" {
		t.Errorf("local fields = %v, want the shared values", local.Fields)
	}
	staging := alice.Effective("staging")
	if staging.Fields["password"] != "staging-pw" || staging.Fields["site"] != "2" || staging.Fields["username"] != "alice" {
		t.Errorf("staging fields = %v, want password and site overridden and username inherited", staging.Fields)
	}
	if staging.FieldTypes["site"] != FieldNumber {
		t.Errorf("staging field types = %v, want the inherited number type", staging.FieldTypes)
	}
	if alice.Fields["password"] != "pw" {
		t.Errorf("Effective changed the stored user: %v", alice.Fields)
	}
	if len(staging.Overrides) != 0 || len(staging.Environments) != 1 || staging.Environments[0] != "staging" {
		t.Errorf("effective user = %+v, want no overrides and one environment", staging)
	}

	bot := multiEnvRester().Users[1]
	if got := bot.Effective("local").Token; got != "pasted" {
		t.Errorf("local token = %q, want the shared one", got)
	}
	if got := bot.Effective("prod").Token; got != "prod-token" {
		t.Errorf("prod token = %q, want the override", got)
	}
}

func TestCanUseByEnvironmentAndType(t *testing.T) {
	r := multiEnvRester()
	alice, bot, idle, apionly := r.Users[0], r.Users[1], r.Users[2], r.Users[3]
	cases := []struct {
		name string
		typ  string
		env  string
		user ResterUser
		want bool
	}{
		{"login user on api", TypeAPI, "local", alice, true},
		{"login user on web", TypeWeb, "local", alice, true},
		{"environment without an origin has no web", TypeWeb, "prod", alice, false},
		{"byot user on api", TypeAPI, "prod", bot, true},
		{"byot user never on web", TypeWeb, "local", bot, false},
		{"unassigned user is unusable", TypeAPI, "local", idle, false},
		{"user missing a field a fill step names", TypeWeb, "local", apionly, false},
		{"user not assigned to this environment", TypeAPI, "staging", bot, false},
		{"unknown environment", TypeAPI, "nowhere", alice, false},
	}
	for _, c := range cases {
		if got := r.CanUse(c.typ, c.env, c.user); got != c.want {
			t.Errorf("%s: CanUse = %v, want %v", c.name, got, c.want)
		}
	}

	noLogin := multiEnvRester()
	noLogin.API.LoginEndpoint = ""
	if noLogin.CanUse(TypeAPI, "local", alice) {
		t.Error("a login user can use the api side of a rester with no login endpoint")
	}
	if !noLogin.CanUse(TypeAPI, "local", bot) {
		t.Error("a byot user cannot use an api side with no login endpoint")
	}
}

func TestResolveFlattensOneEnvironment(t *testing.T) {
	r := multiEnvRester()

	api, err := r.resolve("staging", TypeAPI)
	if err != nil {
		t.Fatal(err)
	}
	if api.Environment != "staging" || api.BaseURL != "https://api.staging.test" || api.Origin != "" || len(api.Login) != 0 {
		t.Errorf("api view = %+v", api)
	}
	if len(api.Users) != 1 || api.Users[0].Name != "alice" || api.Users[0].Fields["password"] != "staging-pw" {
		t.Errorf("staging api users = %+v, want only alice with her staging password", api.Users)
	}
	headers := map[string]string{}
	for _, h := range api.Headers {
		headers[h.Name] = h.Value
	}
	if len(api.Headers) != 2 || headers["X-App"] != "shared" || headers["x-tenant"] != "staging" {
		t.Errorf("headers = %v, want the environment's x-tenant to replace the shared X-Tenant", api.Headers)
	}

	web, err := r.resolve("local", TypeWeb)
	if err != nil {
		t.Fatal(err)
	}
	if web.Origin != "http://localhost:3000" || len(web.Login) != 4 || len(web.Users) != 1 || web.Users[0].Name != "alice" {
		t.Errorf("web view = %+v, want the steps and only alice (the byot and api-only users cannot use the web side)", web)
	}
	if web.TokenPlacement.Name != "Authorization" || web.TokenPath != "token" {
		t.Errorf("web view lost the api token hints: %+v", web)
	}

	for _, c := range []struct{ env, typ, want string }{
		{"nowhere", TypeAPI, "no environment"},
		{"prod", TypeWeb, "no origin"},
		{"local", "ftp", "unknown channel type"},
	} {
		if _, err := r.resolve(c.env, c.typ); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("resolve(%s, %s) = %v, want %q", c.env, c.typ, err, c.want)
		}
	}
	apiOnly := multiEnvRester()
	apiOnly.Web = nil
	if _, err := apiOnly.resolve("local", TypeWeb); err == nil || !strings.Contains(err.Error(), "no web side") {
		t.Errorf("resolve web on an api-only rester = %v", err)
	}
}

func TestValidate(t *testing.T) {
	if err := multiEnvRester().Validate(); err != nil {
		t.Fatalf("the fixture is invalid: %v", err)
	}
	cases := []struct {
		name string
		edit func(r *Rester)
		want string
	}{
		{"no side", func(r *Rester) { r.API, r.Web = nil, nil }, "api side, a web side"},
		{"no environment", func(r *Rester) { r.Environments = nil; r.Users = nil }, "at least one environment"},
		{"duplicate environment", func(r *Rester) { r.Environments[1].Name = "local" }, `environment "local" is listed more than once`},
		{"unnamed environment", func(r *Rester) { r.Environments[1].Name = " " }, "has no name"},
		{"bad base url", func(r *Rester) { r.Environments[0].BaseURL = "localhost:8080" }, "base URL"},
		{"origin with a path", func(r *Rester) { r.Environments[0].Origin = "http://localhost:3000/app" }, "has no path"},
		{"environment header set by the proxy", func(r *Rester) { r.Environments[0].Headers = []Header{{Name: "authorization", Value: "x"}} }, "set by the proxy"},
		{"bad login step action", func(r *Rester) { r.Web.Login[0].Action = "hover" }, "unknown action"},
		{"fill without a field", func(r *Rester) { r.Web.Login[1].Field = "" }, "needs a selector and a field"},
		{"wait with nothing to wait for", func(r *Rester) { r.Web.Login = []LoginStep{{Action: StepWait}} }, "needs a selector or a URL pattern"},
		{"user in a missing environment", func(r *Rester) { r.Users[0].Environments = []string{"local", "mars"} }, `"mars", which does not exist`},
		{"user assigned twice", func(r *Rester) { r.Users[3].Environments = []string{"local", "local"} }, "more than once"},
		{"override for an unassigned environment", func(r *Rester) {
			r.Users[3].Overrides = map[string]UserOverride{"prod": {Fields: map[string]string{"username": "y"}}}
		}, "not assigned to"},
		{"override for a missing environment", func(r *Rester) {
			r.Users[3].Overrides = map[string]UserOverride{"mars": {}}
		}, "does not exist"},
		{"token and fields together", func(r *Rester) { r.Users[0].Token = "t" }, "both a token and login fields"},
		{"override token on a login user", func(r *Rester) {
			r.Users[3].Overrides = map[string]UserOverride{"local": {Token: "t"}}
		}, "both a token and login fields"},
		{"byot user whose only token is an override", func(r *Rester) {
			r.Users[1].Token = ""
		}, `user "bot" is a token user but has no token in environment "local"`},
		{"override breaks a typed field", func(r *Rester) {
			r.Users[0].Overrides["staging"] = UserOverride{Fields: map[string]string{"site": "two"}}
		}, `user "alice" in environment "staging": login field "site" is not a number`},
		{"nameless user", func(r *Rester) { r.Users[2].Name = " " }, "has no name"},
		{"handle collision", func(r *Rester) { r.Users[2].Name = "Alice" }, "both map to the handle"},
	}
	for _, c := range cases {
		r := multiEnvRester()
		c.edit(&r)
		err := r.Validate()
		if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Validate = %v, want an error containing %q", c.name, err, c.want)
		}
	}

	zeroUsers := multiEnvRester()
	zeroUsers.Users = nil
	if err := zeroUsers.Validate(); err != nil {
		t.Errorf("a rester with no users = %v, want it allowed", err)
	}
	unassigned := multiEnvRester()
	if err := unassigned.Validate(); err != nil || len(unassigned.Users[2].Environments) != 0 {
		t.Errorf("an unassigned user = %v, want it allowed", err)
	}
}

func TestWebScopeValidate(t *testing.T) {
	ok := WebScope{Origins: []string{"https://sso.example.test"}, Exclusions: []MethodPath{{Method: "POST", PathPattern: "/api/*"}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid web scope = %v", err)
	}
	for name, w := range map[string]WebScope{
		"origin with a path":  {Origins: []string{"https://sso.example.test/x"}},
		"origin not a url":    {Origins: []string{"sso"}},
		"exclusion no method": {Exclusions: []MethodPath{{PathPattern: "/x"}}},
		"bad pattern":         {Exclusions: []MethodPath{{Method: "GET", PathPattern: "[a"}}},
	} {
		if err := w.Validate(); err == nil || !strings.HasPrefix(err.Error(), "vault: ") {
			t.Errorf("%s: Validate = %v, want a vault error", name, err)
		}
	}
}

func putMultiEnv(t *testing.T) *Service {
	t.Helper()
	svc := openTestService(t)
	if err := svc.PutRester(testRootSecret, "shop", multiEnvRester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	return svc
}

func TestCreateHTTPChannelByEnvironmentAndType(t *testing.T) {
	svc := putMultiEnv(t)
	get := HTTPScope{Methods: []string{"GET"}}

	api, err := svc.CreateHTTPChannel(testRootSecret, "a", "shop", "staging", TypeAPI, get, &WebScope{Origins: []string{"https://x.test"}}, time.Hour)
	if err != nil {
		t.Fatalf("api channel: %v", err)
	}
	if api.Environment != "staging" || api.Type != TypeAPI || api.WebScope != nil || len(api.Scope.Methods) != 1 || api.Rester != "shop" {
		t.Errorf("api channel = %+v, want staging, api, no web scope", api)
	}

	web, err := svc.CreateHTTPChannel(testRootSecret, "w", "shop", "local", TypeWeb, get, &WebScope{Origins: []string{"https://sso.test"}}, time.Hour)
	if err != nil {
		t.Fatalf("web channel: %v", err)
	}
	if web.Type != TypeWeb || web.WebScope == nil || len(web.WebScope.Origins) != 1 || len(web.Scope.Methods) != 0 {
		t.Errorf("web channel = %+v, want a web scope and no api scope", web)
	}
	loaded, err := svc.HTTPChannelScope(web.ID)
	if err != nil || loaded.Type != TypeWeb || loaded.Environment != "local" || loaded.WebScope == nil {
		t.Errorf("stored web channel = %+v, %v", loaded, err)
	}

	for _, c := range []struct{ name, env, typ, want string }{
		{"unknown environment", "mars", TypeAPI, "no environment"},
		{"no address for the type", "prod", TypeWeb, "no origin"},
		{"unknown type", "local", "ftp", "unknown channel type"},
		{"no environment", "", TypeAPI, "needs an environment"},
	} {
		_, err := svc.CreateHTTPChannel(testRootSecret, "x", "shop", c.env, c.typ, get, nil, time.Hour)
		if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: CreateHTTPChannel = %v, want an error containing %q", c.name, err, c.want)
		}
	}
	if _, err := svc.CreateHTTPChannel(testRootSecret, "x", "shop", "local", TypeWeb, get, &WebScope{Origins: []string{"nope"}}, time.Hour); err == nil {
		t.Error("a bad web scope was accepted")
	}

	r := multiEnvRester()
	r.Users = r.Users[3:]
	if err := svc.PutRester(testRootSecret, "thin", r); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateHTTPChannel(testRootSecret, "x", "thin", "local", TypeWeb, get, nil, time.Hour)
	if err == nil || !strings.Contains(err.Error(), "no user of rester thin can use a web channel") {
		t.Errorf("a channel no user could use = %v, want it refused", err)
	}
}

func TestOpenWebChannel(t *testing.T) {
	svc := putMultiEnv(t)
	web, err := svc.CreateHTTPChannel(testRootSecret, "w", "shop", "local", TypeWeb, HTTPScope{}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	api, err := svc.CreateHTTPChannel(testRootSecret, "a", "shop", "local", TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	c, rw, err := svc.OpenWebChannel(web.ID)
	if err != nil {
		t.Fatalf("OpenWebChannel: %v", err)
	}
	if c.Type != TypeWeb || rw.Environment != "local" || rw.Origin != "http://localhost:3000" || len(rw.Login) != 4 || len(rw.Users) != 1 || rw.Users[0].Name != "alice" {
		t.Errorf("web view = %+v", rw)
	}
	if rw.BaseURL != "http://localhost:8080" || rw.TokenPath != "token" || rw.TokenPlacement.Name != "Authorization" {
		t.Errorf("web view api hints = %+v", rw)
	}
	if _, _, err := svc.OpenWebChannel(api.ID); err != ErrNotWebChannel {
		t.Errorf("OpenWebChannel on an api channel = %v, want ErrNotWebChannel", err)
	}

	svc.forgetKey(web.ID)
	if _, _, err := svc.OpenWebChannel(web.ID); err != ErrChannelInactive {
		t.Errorf("OpenWebChannel on an inactive channel = %v, want ErrChannelInactive", err)
	}
	if _, err := svc.ActivateHTTP(testRootSecret, web.ID, -time.Second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.OpenWebChannel(web.ID); err != ErrChannelExpired {
		t.Errorf("OpenWebChannel on an expired channel = %v, want ErrChannelExpired", err)
	}
}

func TestUpdateResterRewrapsEachChannelForItsEnvironment(t *testing.T) {
	svc := putMultiEnv(t)
	local, err := svc.CreateHTTPChannel(testRootSecret, "l", "shop", "local", TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := svc.CreateHTTPChannel(testRootSecret, "s", "shop", "staging", TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var invalidated []ChannelID
	svc.OnInvalidate(func(id ChannelID) { invalidated = append(invalidated, id) })

	r, err := svc.GetRester(testRootSecret, "shop")
	if err != nil {
		t.Fatal(err)
	}
	r.API.LoginEndpoint = "/v2/login"
	r.Users[0].Overrides["staging"] = UserOverride{Fields: map[string]string{"password": "rotated"}}
	if err := svc.UpdateRester(testRootSecret, "shop", r); err != nil {
		t.Fatalf("UpdateRester: %v", err)
	}
	if len(invalidated) != 2 {
		t.Errorf("invalidated %v, want both channels", invalidated)
	}
	_, got, _ := svc.OpenHTTPChannel(staging.ID)
	if got.LoginEndpoint != "/v2/login" || got.Users[0].Fields["password"] != "rotated" || got.BaseURL != "https://api.staging.test" {
		t.Errorf("staging channel = %+v, want the rotated password and its own base URL", got)
	}
	_, got, _ = svc.OpenHTTPChannel(local.ID)
	if got.LoginEndpoint != "/v2/login" || got.Users[0].Fields["password"] != "pw" || got.BaseURL != "http://localhost:8080" {
		t.Errorf("local channel = %+v, want the shared password and its own base URL", got)
	}
}

func TestUpdateResterRefusesRemovingWhatAChannelUses(t *testing.T) {
	svc := putMultiEnv(t)
	staging, err := svc.CreateHTTPChannel(testRootSecret, "staging-api", "shop", "staging", TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	web, err := svc.CreateHTTPChannel(testRootSecret, "local-web", "shop", "local", TypeWeb, HTTPScope{}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	noStaging, _ := svc.GetRester(testRootSecret, "shop")
	noStaging.Environments = []Environment{noStaging.Environments[0], noStaging.Environments[2]}
	noStaging.Users[0].Environments = []string{"local", "prod"}
	delete(noStaging.Users[0].Overrides, "staging")
	err = svc.UpdateRester(testRootSecret, "shop", noStaging)
	if err == nil || !strings.Contains(err.Error(), "staging-api") || strings.Contains(err.Error(), "local-web") {
		t.Fatalf("removing a used environment = %v, want it refused naming staging-api only", err)
	}

	noWeb, _ := svc.GetRester(testRootSecret, "shop")
	noWeb.Web = nil
	err = svc.UpdateRester(testRootSecret, "shop", noWeb)
	if err == nil || !strings.Contains(err.Error(), "local-web") {
		t.Fatalf("removing a used side = %v, want it refused naming local-web", err)
	}

	still, _ := svc.GetRester(testRootSecret, "shop")
	if len(still.Environments) != 3 || still.Web == nil {
		t.Errorf("a refused edit changed the stored rester: %+v", still)
	}
	if _, _, err := svc.OpenHTTPChannel(staging.ID); err != nil {
		t.Errorf("channel after a refused edit: %v", err)
	}
	_ = web
}
