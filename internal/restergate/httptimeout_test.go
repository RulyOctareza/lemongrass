// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func newSlowServer(delay time.Duration) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return httptest.NewServer(mux)
}

func setTimeouts(t *testing.T, login, request time.Duration) {
	t.Helper()
	oldLogin, oldRequest := httpClient.Timeout, RequestTimeout
	httpClient.Timeout, RequestTimeout = login, request
	t.Cleanup(func() { httpClient.Timeout, RequestTimeout = oldLogin, oldRequest })
}

func TestRequestHTTPUpstreamIsNotBoundByTheLoginTimeout(t *testing.T) {
	srv := newSlowServer(400 * time.Millisecond)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"POST"})
	setTimeouts(t, 100*time.Millisecond, 5*time.Second)

	result, err := svc.RequestHTTP(id, "alice", "POST", "/import", []byte("x"), "text/csv", nil)
	if err != nil || result.Status != 200 {
		t.Errorf("RequestHTTP = %+v, %v, want a slow upstream to complete under RequestTimeout", result, err)
	}
}

func TestRequestHTTPGivesUpAfterRequestTimeout(t *testing.T) {
	srv := newSlowServer(time.Second)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"POST"})
	setTimeouts(t, 5*time.Second, 200*time.Millisecond)

	_, err := svc.RequestHTTP(id, "alice", "POST", "/import", []byte("x"), "text/csv", nil)
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Errorf("err = %v, want the request to fail once RequestTimeout passes", err)
	}
}
