package restergate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

type recordedBody struct {
	contentType string
	body        []byte
}

type bodyRecorder struct {
	mu   sync.Mutex
	seen []recordedBody
}

func (r *bodyRecorder) add(req *http.Request) {
	b, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, recordedBody{contentType: req.Header.Get("Content-Type"), body: b})
}

func (r *bodyRecorder) all() []recordedBody {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedBody(nil), r.seen...)
}

func newBodyServer(rec *bodyRecorder) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return httptest.NewServer(mux)
}

func spreadsheetShapedBytes() []byte {
	b := []byte("PK\x03\x04")
	for i := 0; i < 256; i++ {
		b = append(b, byte(i))
	}
	return append(b, bytes.Repeat([]byte{0, 0xff, 0x0d, 0x0a}, 64)...)
}

func TestRequestHTTPSendsBinaryBodyByteForByteWithContentType(t *testing.T) {
	rec := &bodyRecorder{}
	srv := newBodyServer(rec)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"POST"})

	want := spreadsheetShapedBytes()
	result, err := svc.RequestHTTP(id, "alice", "POST", "/import", want, xlsxType, nil)
	if err != nil || result.Status != 200 {
		t.Fatalf("RequestHTTP = %+v, %v", result, err)
	}
	seen := rec.all()
	if len(seen) != 1 || seen[0].contentType != xlsxType || !bytes.Equal(seen[0].body, want) {
		t.Errorf("server saw %d requests, first content type %q, body identical = %v", len(seen), seen[0].contentType, bytes.Equal(seen[0].body, want))
	}
}

func TestRequestHTTPDefaultsToJSONWithoutContentType(t *testing.T) {
	rec := &bodyRecorder{}
	srv := newBodyServer(rec)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"POST"})

	if _, err := svc.RequestHTTP(id, "alice", "POST", "/orders", []byte(`{"a":1}`), "", nil); err != nil {
		t.Fatal(err)
	}
	seen := rec.all()
	if len(seen) != 1 || seen[0].contentType != "application/json" || string(seen[0].body) != `{"a":1}` {
		t.Errorf("seen = %+v, want the inline JSON body with application/json", seen)
	}
}

func TestRequestHTTPRejectsOversizeBodyAndBadContentType(t *testing.T) {
	rec := &bodyRecorder{}
	srv := newBodyServer(rec)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"POST"})

	_, err := svc.RequestHTTP(id, "alice", "POST", "/import", make([]byte, MaxRequestBodyBytes+1), xlsxType, nil)
	if err == nil || !strings.Contains(err.Error(), "25 MiB") {
		t.Errorf("oversize err = %v, want one naming the 25 MiB limit", err)
	}
	if _, err := svc.RequestHTTP(id, "alice", "POST", "/import", make([]byte, MaxRequestBodyBytes), xlsxType, nil); err != nil {
		t.Errorf("a body exactly at the limit: %v", err)
	}

	for _, bad := range []string{"text/plain\r\nX-Injected: 1", "not a media type", "text/", "nonsense"} {
		if _, err := svc.RequestHTTP(id, "alice", "POST", "/import", []byte("x"), bad, nil); err == nil {
			t.Errorf("content type %q was accepted", bad)
		}
	}
	for _, seen := range rec.all() {
		if seen.contentType != xlsxType {
			t.Errorf("a rejected request reached the server with content type %q", seen.contentType)
		}
	}
}

func TestRequestHTTPRetryResendsSameBodyAndContentType(t *testing.T) {
	var logins atomic.Int32
	rec := &bodyRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": fmt.Sprintf("tok-%d", logins.Add(1))})
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		if r.Header.Get("Authorization") != "Bearer tok-2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET", "POST"})

	if _, err := svc.RequestHTTP(id, "alice", "GET", "/ok", nil, "", nil); err != nil {
		t.Fatal(err)
	}
	want := spreadsheetShapedBytes()
	result, err := svc.RequestHTTP(id, "alice", "POST", "/import", want, xlsxType, nil)
	if err != nil || result.Status != 200 {
		t.Fatalf("RequestHTTP = %+v, %v", result, err)
	}
	seen := rec.all()
	if len(seen) != 2 {
		t.Fatalf("server saw %d import requests, want the stale attempt and the retry", len(seen))
	}
	for i, s := range seen {
		if s.contentType != xlsxType || !bytes.Equal(s.body, want) {
			t.Errorf("attempt %d: content type %q, body identical = %v", i, s.contentType, bytes.Equal(s.body, want))
		}
	}
}
