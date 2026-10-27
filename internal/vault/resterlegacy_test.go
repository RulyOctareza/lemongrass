// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

const legacyDomainJSON = `{"BaseURL":"https://api.example.test/v1","LoginEndpoint":"/login","TokenPath":"access_token","TTLOrigin":"","FixedTTLSeconds":60,` +
	`"TokenPlacement":{"Kind":"header","Name":"Authorization","Prefix":"Bearer "},` +
	`"Users":[{"Name":"alice","Fields":{"u":"alice"},"FieldTypes":null,"Token":"","Tags":["admin"]},{"Name":"bot","Token":"pasted"}],` +
	`"Headers":[{"Name":"X-Tenant","Value":"t1"}]}`

func TestStoredOldDomainReadsAsRester(t *testing.T) {
	svc := openTestService(t)
	rootKey, err := DeriveKey(testRootSecret, svc.rootSalt)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.creds.Put("domains-staging", rootKey, []byte(legacyDomainJSON)); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetRester(testRootSecret, "staging")
	if err != nil {
		t.Fatalf("GetRester on an old domain: %v", err)
	}
	if got.API == nil || got.Web != nil {
		t.Fatalf("sides = api %v web %v, want an api side only", got.API, got.Web)
	}
	if got.API.LoginEndpoint != "/login" || got.API.TokenPath != "access_token" || got.API.FixedTTLSeconds != 60 || got.API.TokenPlacement.Prefix != "Bearer " {
		t.Errorf("api side = %+v, want the domain's login settings", got.API)
	}
	if len(got.API.Headers) != 1 || got.API.Headers[0].Name != "X-Tenant" {
		t.Errorf("shared headers = %v, want the domain's headers moved to the api side", got.API.Headers)
	}
	if len(got.Environments) != 1 || got.Environments[0].Name != DefaultEnvironment || got.Environments[0].BaseURL != "https://api.example.test/v1" || len(got.Environments[0].Headers) != 0 {
		t.Errorf("environments = %+v, want one default environment holding the base URL", got.Environments)
	}
	if len(got.Users) != 2 || got.Users[0].Tags[0] != "admin" || got.Users[1].Token != "pasted" {
		t.Fatalf("users = %+v, want both users kept", got.Users)
	}
	for _, u := range got.Users {
		if len(u.Environments) != 1 || u.Environments[0] != DefaultEnvironment {
			t.Errorf("user %s environments = %v, want assigned to default", u.Name, u.Environments)
		}
	}
	if err := got.Validate(); err != nil {
		t.Errorf("a migrated rester fails validation: %v", err)
	}

	names, err := svc.ListResters()
	if err != nil || len(names) != 1 || names[0] != "staging" {
		t.Errorf("ListResters = %v, %v, want the old domain listed", names, err)
	}
}

func TestOldDomainSavesInNewShape(t *testing.T) {
	svc := openTestService(t)
	rootKey, _ := DeriveKey(testRootSecret, svc.rootSalt)
	if err := svc.creds.Put("domains-staging", rootKey, []byte(legacyDomainJSON)); err != nil {
		t.Fatal(err)
	}
	r, err := svc.GetRester(testRootSecret, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateRester(testRootSecret, "staging", r); err != nil {
		t.Fatalf("UpdateRester: %v", err)
	}
	raw, err := svc.creds.Get("domains-staging", rootKey)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["Environments"]; !ok {
		t.Errorf("record keys after save = %v, want the new shape", keys)
	}
}

func TestOldChannelKeepsWorking(t *testing.T) {
	svc := openTestService(t)
	id, err := NewChannelID()
	if err != nil {
		t.Fatal(err)
	}
	salt, err := NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	key, err := DeriveKey(testRootSecret, salt)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.channels.Put(string(id), key, []byte(legacyDomainJSON)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	meta, err := json.Marshal(map[string]any{
		"ID": id, "Name": "old", "Domain": "staging",
		"Scope": HTTPScope{Methods: []string{"GET"}}, "Salt": salt,
		"CreatedAt": now, "ExpiresAt": now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.httpMetaPath(id), meta, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := svc.HTTPChannelScope(id)
	if err != nil {
		t.Fatal(err)
	}
	if c.Rester != "staging" || c.Environment != DefaultEnvironment || c.Type != TypeAPI || c.WebScope != nil {
		t.Errorf("old channel metadata = %+v, want rester staging, environment default, type api", c)
	}
	out, _ := json.Marshal(c)
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(out, &keys)
	if _, ok := keys["Domain"]; ok {
		t.Errorf("channel encodes a Domain key: %s", out)
	}

	if _, err := svc.ActivateHTTP(testRootSecret, id, time.Hour); err != nil {
		t.Fatalf("ActivateHTTP: %v", err)
	}
	c, res, err := svc.OpenHTTPChannel(id)
	if err != nil {
		t.Fatalf("OpenHTTPChannel on an old channel: %v", err)
	}
	if res.Environment != DefaultEnvironment || res.BaseURL != "https://api.example.test/v1" || res.LoginEndpoint != "/login" || len(res.Users) != 2 {
		t.Errorf("old copy = %+v, want the flat domain read as the default environment", res)
	}
	if len(res.Headers) != 1 || res.Headers[0].Value != "t1" || res.TokenPlacement.Name != "Authorization" {
		t.Errorf("old copy headers or placement = %+v", res)
	}
}

func TestOldChannelCopyIsRewrappedOnUpdate(t *testing.T) {
	svc := openTestService(t)
	rootKey, _ := DeriveKey(testRootSecret, svc.rootSalt)
	if err := svc.creds.Put("domains-staging", rootKey, []byte(legacyDomainJSON)); err != nil {
		t.Fatal(err)
	}
	r, err := svc.GetRester(testRootSecret, "staging")
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateHTTPChannel(testRootSecret, "ch", "staging", DefaultEnvironment, TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r.Environments[0].BaseURL = "https://moved.example.test"
	if err := svc.UpdateRester(testRootSecret, "staging", r); err != nil {
		t.Fatal(err)
	}
	_, res, err := svc.OpenHTTPChannel(c.ID)
	if err != nil || res.BaseURL != "https://moved.example.test" {
		t.Errorf("channel after update = %+v, %v, want the new base URL", res, err)
	}
}

func TestOldDomainTokenUserKeepsTokenAndDropsFields(t *testing.T) {
	svc := openTestService(t)
	rootKey, _ := DeriveKey(testRootSecret, svc.rootSalt)
	old := `{"BaseURL":"https://api.example.test","Users":[{"Name":"both","Fields":{"u":"x"},"FieldTypes":{"u":"number"},"Token":"pasted"}]}`
	if err := svc.creds.Put("domains-staging", rootKey, []byte(old)); err != nil {
		t.Fatal(err)
	}
	r, err := svc.GetRester(testRootSecret, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if u := r.Users[0]; u.Token != "pasted" || len(u.Fields) != 0 || len(u.FieldTypes) != 0 {
		t.Errorf("migrated user = %+v, want the token kept and the login fields dropped", u)
	}
	if err := svc.UpdateRester(testRootSecret, "staging", r); err != nil {
		t.Errorf("saving a migrated token user: %v", err)
	}
}

func TestSaveErrorNamesResterEnvironmentAndField(t *testing.T) {
	svc := openTestService(t)
	r := NewAPIRester("http://user:pw@api.example.test", ResterAPI{}, nil)
	err := svc.PutRester(testRootSecret, "shop", r)
	for _, want := range []string{`rester "shop"`, `environment "default"`, "base URL"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("PutRester = %v, want it to contain %q", err, want)
		}
	}
	if !strings.HasPrefix(err.Error(), "vault: ") || strings.Count(err.Error(), "vault: ") != 1 {
		t.Errorf("error = %q, want one vault prefix", err)
	}
}
