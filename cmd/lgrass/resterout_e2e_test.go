package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func downloadBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*37 + i>>10)
	}
	return b
}

// startDownloadChannel wires a CLI-facing agent client to a channel on a domain served by mux, which gets a /login route added.
func startDownloadChannel(t *testing.T, mux *http.ServeMux) (*agent.Client, string) {
	t.Helper()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	vaultClient, agentClient := startStack(t)
	const passphrase = "correct horse"
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(passphrase, "staging", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(passphrase, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET", "POST"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := agentClient.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return agentClient, shortID
}

func runResterArgs(t *testing.T, client *agent.Client, args ...string) (any, error) {
	t.Helper()
	cmd, err := parseResterArgs(args)
	if err != nil {
		t.Fatalf("parseResterArgs(%v): %v", args, err)
	}
	return execRester(client, cmd)
}

func TestOutSavesALargeFileThroughTheWholeChain(t *testing.T) {
	want := downloadBytes(48<<20 + 5)
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		w.Write(want)
	})
	client, shortID := startDownloadChannel(t, mux)
	target := filepath.Join(t.TempDir(), "export.bin")

	out, err := runResterArgs(t, client, shortID, "get", "/export", "--out", target)
	if err != nil {
		t.Fatalf("execRester: %v", err)
	}
	res, ok := out.(downloadResult)
	if !ok || res.SavedTo != target || res.Size != int64(len(want)) || res.Status != 200 {
		t.Fatalf("result = %+v, want a saved result for %s of %d bytes", out, target, len(want))
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "body") {
		t.Errorf("printed result %s carries a body field", raw)
	}
	got, err := os.ReadFile(target)
	if err != nil || sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("saved file differs from the %d bytes served (read err %v)", len(want), err)
	}
}

func TestOutIntoADirectoryUsesTheServersFileName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../../q3 report.xlsx"`)
		w.Write([]byte("sheet"))
	})
	client, shortID := startDownloadChannel(t, mux)
	dir := t.TempDir()

	out, err := runResterArgs(t, client, shortID, "get", "/export", "--out", dir)
	if err != nil {
		t.Fatalf("execRester: %v", err)
	}
	want := filepath.Join(dir, "q3 report.xlsx")
	if res, ok := out.(downloadResult); !ok || res.SavedTo != want {
		t.Fatalf("result = %+v, want saved_to %s", out, want)
	}
	if got, _ := os.ReadFile(want); string(got) != "sheet" {
		t.Errorf("file = %q, want sheet", got)
	}
}

func TestOutExistingFileMakesNoRequestWithoutConfirm(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("new"))
	})
	client, shortID := startDownloadChannel(t, mux)
	target := filepath.Join(t.TempDir(), "export.bin")
	os.WriteFile(target, []byte("old"), 0o644)

	_, err := runResterArgs(t, client, shortID, "get", "/export", "--out", target)
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("err = %v, want the warning naming --confirm", err)
	}
	if hits.Load() != 0 {
		t.Errorf("upstream saw %d requests, want none before the confirmation", hits.Load())
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Errorf("file = %q, want it untouched", got)
	}

	if _, err := runResterArgs(t, client, shortID, "get", "/export", "--out", target, "--confirm"); err != nil {
		t.Fatalf("with --confirm: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("file = %q, want it replaced", got)
	}
}

func TestOutNon2xxIsPrintedInlineAndWritesNothing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no such export"}`))
	})
	client, shortID := startDownloadChannel(t, mux)
	dir := t.TempDir()
	target := filepath.Join(dir, "export.bin")

	out, err := runResterArgs(t, client, shortID, "get", "/export", "--out", target)
	if err != nil {
		t.Fatalf("execRester: %v", err)
	}
	res, ok := out.(restergate.HTTPResult)
	if !ok || res.Status != 404 {
		t.Fatalf("result = %+v, want the 404 as an ordinary inline result", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("directory holds %v, want nothing written", entries)
	}
}

func TestOutFollowsAPresignedRedirectToAnotherHost(t *testing.T) {
	var sawAuth atomic.Value
	sawAuth.Store("unset")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth.Store(r.Header.Get("Authorization"))
		w.Write([]byte("from the storage host"))
	}))
	defer other.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/bucket/presigned.txt?sig=abc", http.StatusFound)
	})
	client, shortID := startDownloadChannel(t, mux)
	dir := t.TempDir()

	out, err := runResterArgs(t, client, shortID, "get", "/export", "--out", dir)
	if err != nil {
		t.Fatalf("execRester: %v", err)
	}
	res := out.(downloadResult)
	if res.SavedTo != filepath.Join(dir, "presigned.txt") {
		t.Errorf("saved_to = %q, want the file named from the final URL", res.SavedTo)
	}
	if got, _ := os.ReadFile(res.SavedTo); string(got) != "from the storage host" {
		t.Errorf("file = %q", got)
	}
	if sawAuth.Load() != "" {
		t.Errorf("the storage host saw Authorization %q, want none", sawAuth.Load())
	}
}

func TestOutPostSendsItsBodyAndSavesTheReply(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/render", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Write([]byte("rendered:" + string(b)))
	})
	client, shortID := startDownloadChannel(t, mux)
	target := filepath.Join(t.TempDir(), "render.txt")

	if _, err := runResterArgs(t, client, shortID, "post", "/render", "--body", `{"a":1}`, "--out", target); err != nil {
		t.Fatalf("execRester: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != `rendered:{"a":1}` {
		t.Errorf("file = %q, want the POST reply", got)
	}
}

func TestOutCutUpstreamLeavesTheExistingFileAndNoTemp(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5000000")
		w.Write(downloadBytes(1000))
		w.(http.Flusher).Flush()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	client, shortID := startDownloadChannel(t, mux)
	dir := t.TempDir()
	target := filepath.Join(dir, "export.bin")
	os.WriteFile(target, []byte("precious"), 0o644)

	_, err := runResterArgs(t, client, shortID, "get", "/export", "--out", target, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("err = %v, want the failed download reported", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "precious" {
		t.Errorf("file = %q, want it untouched", got)
	}
	noTemps(t, dir)
}

func TestOutStalledUpstreamIsGivenUpOn(t *testing.T) {
	old := restergate.RequestTimeout
	restergate.RequestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { restergate.RequestTimeout = old })

	mux := http.NewServeMux()
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
	})
	client, shortID := startDownloadChannel(t, mux)
	dir := t.TempDir()

	start := time.Now()
	_, err := runResterArgs(t, client, shortID, "get", "/export", "--out", filepath.Join(dir, "export.bin"))
	if err == nil || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("err = %v, want the stalled download reported", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %s to give up, want about the idle timeout", time.Since(start))
	}
	noTemps(t, dir)
}
