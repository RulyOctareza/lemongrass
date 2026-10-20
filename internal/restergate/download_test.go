package restergate

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func aliceOnly() []vault.ResterUser {
	return []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}
}

func newDownloadServer(t *testing.T, file http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", file)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + i>>8)
	}
	return b
}

func TestDownloadHTTPStreamsBodyLargerThanTheInlineLimit(t *testing.T) {
	want := patternBytes(5 << 20)
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="report.xlsx"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		w.Write(want)
	})
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})

	dl, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	if dl.Inline != nil || dl.Body == nil {
		t.Fatalf("Inline = %v, Body = %v, want a streamed body", dl.Inline, dl.Body)
	}
	defer dl.Body.Close()
	if dl.Status != 200 || dl.Name != "report.xlsx" || dl.Length != int64(len(want)) || dl.URL != srv.URL+"/export" {
		t.Errorf("download = %+v, want status 200, name report.xlsx, length %d, the requested URL", dl, len(want))
	}
	got, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("body of %d bytes differs from the %d bytes served", len(got), len(want))
	}
}

func TestDownloadHTTPNon2xxComesBackInline(t *testing.T) {
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no such report"})
	})
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})

	dl, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	if dl.Body != nil || dl.Inline == nil || dl.Inline.Status != 404 || dl.Status != 404 {
		t.Fatalf("download = %+v, want a 404 returned inline with no body stream", dl)
	}
	if body, _ := dl.Inline.Body.(map[string]any); body["error"] != "no such report" {
		t.Errorf("inline body = %v, want the error JSON", dl.Inline.Body)
	}
}

func TestDownloadHTTPCrossHostRedirectDropsTheToken(t *testing.T) {
	placement := vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "}
	svc, id, seen := newRedirectFixture(t, placement, func(other *httptest.Server) string { return other.URL + "/presigned/file.bin?sig=abc" })

	dl, err := svc.DownloadHTTP(id, "alice", "GET", "/download", nil, "", nil)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	defer dl.Body.Close()
	if dl.Name != "file.bin" {
		t.Errorf("Name = %q, want the last segment of the final URL path", dl.Name)
	}
	if len(*seen) != 1 || (*seen)[0].header.Get("Authorization") != "" {
		t.Errorf("the other host saw %+v, want one request with no Authorization header", *seen)
	}
	if strings.Contains(dl.URL, "sig=") {
		t.Errorf("URL = %q, want the requested URL and not the redirect target", dl.URL)
	}
}

func TestDownloadHTTPRetriesAStaleToken(t *testing.T) {
	var logins atomic.Int32
	revoked := map[string]bool{}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": fmt.Sprintf("tok%d", logins.Add(1))})
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		if revoked[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("contents"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})

	first, err := svc.DownloadHTTP(id, "alice", "GET", "/file", nil, "", nil)
	if err != nil {
		t.Fatalf("first DownloadHTTP: %v", err)
	}
	first.Body.Close()
	revoked["tok1"] = true

	second, err := svc.DownloadHTTP(id, "alice", "GET", "/file", nil, "", nil)
	if err != nil {
		t.Fatalf("second DownloadHTTP: %v", err)
	}
	defer second.Body.Close()
	got, _ := io.ReadAll(second.Body)
	if second.Status != 200 || string(got) != "contents" || logins.Load() != 2 {
		t.Errorf("status = %d, body = %q, logins = %d, want a re-login and a 200", second.Status, got, logins.Load())
	}
}

func TestDownloadHTTPRespectsChannelScope(t *testing.T) {
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) })
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"POST"})

	if _, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil); err == nil || !strings.Contains(err.Error(), "does not grant") {
		t.Errorf("err = %v, want a scope refusal", err)
	}
}

func TestDownloadName(t *testing.T) {
	cases := []struct {
		name        string
		disposition string
		finalPath   string
		want        string
	}{
		{"disposition filename", `attachment; filename="q3 report.xlsx"`, "/x/y", "q3 report.xlsx"},
		{"disposition path is reduced to its base", `attachment; filename="../../etc/passwd"`, "/x/y", "passwd"},
		{"backslash path is reduced to its base", `attachment; filename="..\\..\\win.ini"`, "/x/y", "win.ini"},
		{"no disposition uses the url segment", "", "/files/2024/data.csv", "data.csv"},
		{"dot dot filename falls back to the url", `attachment; filename=".."`, "/files/data.csv", "data.csv"},
		{"root path has no name", "", "/", ""},
		{"empty path has no name", "", "", ""},
	}
	for _, c := range cases {
		h := http.Header{}
		if c.disposition != "" {
			h.Set("Content-Disposition", c.disposition)
		}
		if got := downloadName(h, c.finalPath); got != c.want {
			t.Errorf("%s: downloadName = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDownloadHTTPGivesUpWhenHeadersNeverArrive(t *testing.T) {
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(600 * time.Millisecond) })
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})
	setTimeouts(t, 5*time.Second, 150*time.Millisecond)

	_, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "nothing received") {
		t.Errorf("err = %v, want the header wait to end with a nothing-received error", err)
	}
}

func TestDownloadHTTPStalledBodyEndsAfterTheIdleTimeout(t *testing.T) {
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first bytes"))
		w.(http.Flusher).Flush()
		time.Sleep(time.Second)
	})
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})
	setTimeouts(t, 5*time.Second, 200*time.Millisecond)

	dl, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	defer dl.Body.Close()
	_, err = io.ReadAll(dl.Body)
	if err == nil || !strings.Contains(err.Error(), "nothing received") {
		t.Errorf("err = %v, want the stalled body to end with a nothing-received error", err)
	}
}

func TestDownloadHTTPSlowSteadyBodyOutlivesTheRequestTimeout(t *testing.T) {
	srv := newDownloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 8; i++ {
			w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	})
	svc, id := setUpHTTPChannel(t, srv, aliceOnly(), []string{"GET"})
	setTimeouts(t, 5*time.Second, 200*time.Millisecond)

	dl, err := svc.DownloadHTTP(id, "alice", "GET", "/export", nil, "", nil)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	defer dl.Body.Close()
	got, err := io.ReadAll(dl.Body)
	if err != nil || len(got) != 40 {
		t.Errorf("read %d bytes, err = %v, want all 40 bytes although the whole body took longer than RequestTimeout", len(got), err)
	}
}
