// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package gatekeeper

import (
	"errors"
	"testing"

	"github.com/faizalv/lemongrass/internal/restergate"
)

func TestTestResterLoginResultCarriesStatusAndBody(t *testing.T) {
	got := testResterLoginResult(&restergate.LoginStatusError{Status: 422, Body: []byte(`{"detail":"bad"}`)})
	want := testResterLoginResultPayload{Error: "restergate: login returned status 422", Status: 422, Body: `{"detail":"bad"}`}
	if got != want {
		t.Errorf("testResterLoginResult(status error) = %+v, want %+v", got, want)
	}
}

func TestTestResterLoginResultOtherErrorHasNoBody(t *testing.T) {
	got := testResterLoginResult(errors.New("restergate: logging in: connection refused"))
	if got.Error == "" || got.Status != 0 || got.Body != "" {
		t.Errorf("testResterLoginResult(plain error) = %+v, want the message only", got)
	}
	if got := testResterLoginResult(nil); got != (testResterLoginResultPayload{}) {
		t.Errorf("testResterLoginResult(nil) = %+v, want empty", got)
	}
}
