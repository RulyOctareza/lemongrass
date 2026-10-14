package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

// startTokenCountChannel serves a token counting endpoint that, like the Anthropic API, wants x-api-key and anthropic-version, behind a domain holding domainHeaders.
func startTokenCountChannel(t *testing.T, domainHeaders []vault.Header) (*agent.Client, string, *[]http.Header) {
	t.Helper()
	var seen []http.Header
	upstream := http.NewServeMux()
	upstream.HandleFunc("/v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Api-Key") != "sk-test" || r.Header.Get("Anthropic-Version") != "2023-06-01" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "anthropic-version: header is required"})
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"input_tokens": 12})
	})
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)

	vaultClient, agentClient := startStack(t)
	const passphrase = "correct horse"
	domain := vault.Domain{
		BaseURL:        srv.URL,
		TokenPlacement: vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "x-api-key"},
		Users:          []vault.DomainUser{{Name: "key", Token: "sk-test"}},
		Headers:        domainHeaders,
	}
	if err := vaultClient.PutDomain(passphrase, "anthropic", domain); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(passphrase, "ch", "anthropic", vault.HTTPScope{Methods: []string{"POST"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := agentClient.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return agentClient, shortID, &seen
}

const countBody = `{"model": "claude-sonnet-5-5", "messages": [{"role": "user", "content": "Hello"}]}`

func checkTokenCount(t *testing.T, result any, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("rester: %v", err)
	}
	r, ok := result.(restergate.HTTPResult)
	if !ok || r.Status != 200 {
		t.Fatalf("result = %+v, want status 200", result)
	}
	if body, ok := r.Body.(map[string]any); !ok || body["input_tokens"] != float64(12) {
		t.Errorf("body = %v, want input_tokens 12", r.Body)
	}
}

func TestDomainHeaderReachesTheAPIThroughTheWholeChain(t *testing.T) {
	client, shortID, seen := startTokenCountChannel(t, []vault.Header{{Name: "anthropic-version", Value: "2023-06-01"}})
	result, err := runResterArgs(t, client, shortID, "post", "/v1/messages/count_tokens", "--body", countBody, "--header", "anthropic-beta: token-counting")
	checkTokenCount(t, result, err)
	if got := (*seen)[0].Get("Anthropic-Beta"); got != "token-counting" {
		t.Errorf("anthropic-beta = %q, want the --header value", got)
	}
}

func TestCallHeaderReachesTheAPIThroughTheWholeChain(t *testing.T) {
	client, shortID, _ := startTokenCountChannel(t, nil)
	result, err := runResterArgs(t, client, shortID, "post", "/v1/messages/count_tokens", "--body", countBody)
	if err != nil || result.(restergate.HTTPResult).Status != 400 {
		t.Fatalf("without anthropic-version: %+v, %v, want status 400", result, err)
	}
	result, err = runResterArgs(t, client, shortID, "post", "/v1/messages/count_tokens", "--body", countBody, "--header", "anthropic-version: 2023-06-01")
	checkTokenCount(t, result, err)
}

func TestCallHeaderReachesTheAPIOnADownload(t *testing.T) {
	client, shortID, _ := startTokenCountChannel(t, nil)
	out := filepath.Join(t.TempDir(), "count.json")
	if _, err := runResterArgs(t, client, shortID, "post", "/v1/messages/count_tokens", "--body", countBody, "--header", "anthropic-version: 2023-06-01", "--out", out); err != nil {
		t.Fatalf("rester --out: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(got), `"input_tokens":12`) {
		t.Errorf("saved file = %q, %v, want the token count", got, err)
	}
}

func TestRefusedCallHeaderThroughTheWholeChain(t *testing.T) {
	client, shortID, seen := startTokenCountChannel(t, []vault.Header{{Name: "anthropic-version", Value: "2023-06-01"}})
	cases := map[string]string{
		"x-api-key: mine":               `header "x-api-key" is set by the proxy`,
		"Anthropic-Version: 2099-01-01": `header "Anthropic-Version" is set by the domain`,
	}
	for header, want := range cases {
		_, err := runResterArgs(t, client, shortID, "post", "/v1/messages/count_tokens", "--body", countBody, "--header", header)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("--header %q: err = %v, want one containing %q", header, err, want)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("upstream saw %d calls, want none for refused headers", len(*seen))
	}
}
