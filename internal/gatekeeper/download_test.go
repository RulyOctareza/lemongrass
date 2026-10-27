// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

func startDownloadStack(t *testing.T, handler http.HandlerFunc, methods ...string) (*Client, vault.ChannelID) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"tok"}`))
	})
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	backend, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	const rootSecret = "root-secret"
	if err := backend.SetPassphrase(rootSecret); err != nil {
		t.Fatal(err)
	}
	domain := newEditableDomain(srv.URL, twoLoginUsers()[:1])
	if err := backend.PutRester(rootSecret, "d", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	if len(methods) == 0 {
		methods = []string{"GET"}
	}
	c, err := backend.CreateHTTPChannel(rootSecret, "ch", "d", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: methods}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	sock := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	go Serve(backend, l, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { l.Close() })
	return &Client{SocketPath: sock}, c.ID
}

func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + i>>8)
	}
	return b
}

func TestDownloadStreamsALargeBodyIntact(t *testing.T) {
	want := patternBytes(9<<20 + 123)
	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="big.bin"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		w.Write(want)
	})

	res, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	if res.Stream == nil || res.Body == nil || res.Inline != nil {
		t.Fatalf("result = %+v, want a stream", res)
	}
	defer res.Body.Close()
	if res.Stream.Status != 200 || res.Stream.Name != "big.bin" || res.Stream.Length != int64(len(want)) {
		t.Errorf("stream = %+v, want status 200, name big.bin, length %d", res.Stream, len(want))
	}
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("body of %d bytes differs from the %d served", len(got), len(want))
	}
}

func TestDownloadEmptyBodyStillStreams(t *testing.T) {
	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	res, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil || res.Body == nil {
		t.Fatalf("result = %+v, err = %v, want a stream", res, err)
	}
	defer res.Body.Close()
	if got, err := io.ReadAll(res.Body); err != nil || len(got) != 0 {
		t.Errorf("body = %d bytes, err = %v, want an empty body", len(got), err)
	}
}

func TestDownloadNon2xxIsInline(t *testing.T) {
	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"nope"}`))
	})
	res, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	if res.Inline == nil || res.Inline.Status != 403 || res.Body != nil || res.Stream != nil {
		t.Errorf("result = %+v, want a 403 inline", res)
	}
}

func TestDownloadScopeRefusalIsAnError(t *testing.T) {
	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {}, "POST")
	_, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	var remote RemoteError
	if !errors.As(err, &remote) || !strings.Contains(err.Error(), "does not grant") {
		t.Errorf("err = %v, want the daemon's scope refusal as a RemoteError", err)
	}
}

func TestDownloadCutUpstreamIsAnErrorNotAShortFile(t *testing.T) {
	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(patternBytes(1000))
		w.(http.Flusher).Flush()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	res, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err == nil {
		t.Fatalf("read %d bytes with no error, want the cut body reported", len(got))
	}
}

func TestDownloadStalledUpstreamEndsAfterTheIdleTimeout(t *testing.T) {
	old := restergate.RequestTimeout
	restergate.RequestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { restergate.RequestTimeout = old })

	client, id := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
	})
	res, err := client.RequestHTTPDownload(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTPDownload: %v", err)
	}
	defer res.Body.Close()
	start := time.Now()
	_, err = io.ReadAll(res.Body)
	// The daemon's idle timer and the client's read deadline are the same length here, so either one may end the read first.
	if err == nil || !(strings.Contains(err.Error(), "nothing received") || strings.Contains(err.Error(), "i/o timeout")) {
		t.Errorf("err = %v, want the stalled download to end with a timeout error", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %s to give up, want about the idle timeout", time.Since(start))
	}
}

func TestWriteDownloadReportsAnAbortAsAnError(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		WriteDownload(server, DownloadResult{Stream: &StreamMeta{Status: 200}, Body: io.NopCloser(io.MultiReader(strings.NewReader("part"), errReader{errors.New("upstream reset")}))}, time.Second, nil)
	}()
	res, err := ReadDownload(client, time.Second)
	if err != nil {
		t.Fatalf("ReadDownload: %v", err)
	}
	got, err := io.ReadAll(res.Body)
	if string(got) != "part" || err == nil || err.Error() != "upstream reset" {
		t.Errorf("read %q, err = %v, want the bytes before the failure and the abort message", got, err)
	}
}

func TestReadDownloadReportsAMissingEndMarker(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		server.Write([]byte(`{"ok":true,"payload":{"stream":{"status":200,"headers":null,"url":"","name":"","length":-1}}}` + "\n"))
		server.Write([]byte{0, 0, 0, 3, 'a', 'b', 'c'})
	}()
	res, err := ReadDownload(client, time.Second)
	if err != nil {
		t.Fatalf("ReadDownload: %v", err)
	}
	got, err := io.ReadAll(res.Body)
	if string(got) != "abc" || err == nil || !strings.Contains(err.Error(), "end marker") {
		t.Errorf("read %q, err = %v, want the bytes and an end-marker error", got, err)
	}
}

func TestReadDownloadRejectsAnOversizedFrame(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		server.Write([]byte(`{"ok":true,"payload":{"stream":{"status":200,"headers":null,"url":"","name":"","length":-1}}}` + "\n"))
		server.Write([]byte{0x7f, 0xff, 0xff, 0xff})
	}()
	res, err := ReadDownload(client, time.Second)
	if err != nil {
		t.Fatalf("ReadDownload: %v", err)
	}
	if _, err := io.ReadAll(res.Body); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("err = %v, want the oversized frame refused", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
