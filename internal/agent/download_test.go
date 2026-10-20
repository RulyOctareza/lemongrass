package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func startDownloadAgent(t *testing.T, handler http.HandlerFunc) (*Client, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	vaultClient := startTestVault(t)
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(testRootSecret, "staging", domain.Rester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	c, err := vaultClient.CreateHTTPChannel(testRootSecret, "test-channel", "staging", vault.DefaultEnvironment, vault.TypeAPI, fullHTTPScope(), nil, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}
	agentClient := startTestAgent(t, vaultClient)
	shortID, err := agentClient.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	return agentClient, shortID
}

func TestIPCRequestHTTPDownloadStreamsAFileIntact(t *testing.T) {
	want := make([]byte, 6<<20+77)
	for i := range want {
		want[i] = byte(i*13 + i>>9)
	}
	client, shortID := startDownloadAgent(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="export.xlsx"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		w.Write(want)
	})

	res, err := client.RequestHTTPDownload(shortID, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	if res.Stream == nil || res.Body == nil {
		t.Fatalf("result = %+v, want a stream", res)
	}
	defer res.Body.Close()
	if res.Stream.Name != "export.xlsx" || res.Stream.Length != int64(len(want)) {
		t.Errorf("stream = %+v, want name export.xlsx and length %d", res.Stream, len(want))
	}
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("body of %d bytes differs from the %d served", len(got), len(want))
	}
}

func TestIPCRequestHTTPDownloadNon2xxIsInline(t *testing.T) {
	client, shortID := startDownloadAgent(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"gone"}`))
	})
	res, err := client.RequestHTTPDownload(shortID, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	if res.Inline == nil || res.Inline.Status != 404 || res.Body != nil {
		t.Errorf("result = %+v, want a 404 inline", res)
	}
}

func TestIPCRequestHTTPDownloadUnknownShortID(t *testing.T) {
	client := startTestAgent(t, startTestVault(t))
	_, err := client.RequestHTTPDownload("BOGUS1", "alice", "GET", "/export", nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), ErrNoSuchChannel.Error()) {
		t.Errorf("err = %v, want %v", err, ErrNoSuchChannel)
	}
}

func TestIPCRequestHTTPDownloadCutUpstreamIsAnError(t *testing.T) {
	client, shortID := startDownloadAgent(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(make([]byte, 500))
		w.(http.Flusher).Flush()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	res, err := client.RequestHTTPDownload(shortID, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	defer res.Body.Close()
	if got, err := io.ReadAll(res.Body); err == nil {
		t.Errorf("read %d bytes with no error, want the cut download reported", len(got))
	}
}
