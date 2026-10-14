package restergate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

type fileRedirectFixture struct {
	svc      *testSvc
	id       vault.ChannelID
	otherRec *bodyRecorder
	finalRec *bodyRecorder
}

func newFileRedirectFixture(t *testing.T) *fileRedirectFixture {
	t.Helper()
	otherRec, finalRec := &bodyRecorder{}, &bodyRecorder{}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherRec.add(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"from": "other"})
	}))
	t.Cleanup(other.Close)

	var base *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/to-other", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/landing", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/to-final", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base.URL+"/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		finalRec.add(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	base = httptest.NewServer(mux)
	t.Cleanup(base.Close)

	svc := newTestService(t)
	if err := svc.SetPassphrase("root-secret"); err != nil {
		t.Fatal(err)
	}
	domain := newEditableDomain(base.URL, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}})
	if err := svc.PutDomain("root-secret", "d", domain); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateHTTPChannel("root-secret", "ch", "d", vault.HTTPScope{Methods: []string{"POST"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &fileRedirectFixture{svc: svc, id: c.ID, otherRec: otherRec, finalRec: finalRec}
}

func TestFileBodyIsNeverResentToAnotherHost(t *testing.T) {
	f := newFileRedirectFixture(t)
	_, err := f.svc.RequestHTTP(f.id, "alice", "POST", "/to-other", spreadsheetShapedBytes(), xlsxType, nil)
	if err == nil || !strings.Contains(err.Error(), "refusing to resend the file") {
		t.Fatalf("err = %v, want a refusal to resend the file", err)
	}
	if n := len(f.otherRec.all()); n != 0 {
		t.Errorf("the other host received %d requests, want none", n)
	}
}

func TestFileBodyFollowsASameHostRedirectWithItsBytes(t *testing.T) {
	f := newFileRedirectFixture(t)
	want := spreadsheetShapedBytes()
	result, err := f.svc.RequestHTTP(f.id, "alice", "POST", "/to-final", want, xlsxType, nil)
	if err != nil || result.Status != 200 {
		t.Fatalf("RequestHTTP = %+v, %v", result, err)
	}
	seen := f.finalRec.all()
	if len(seen) != 1 || seen[0].contentType != xlsxType || !bytes.Equal(seen[0].body, want) {
		t.Errorf("final host saw %d requests, want one carrying the same type and bytes", len(seen))
	}
}

func TestInlineJSONBodyKeepsTheExistingCrossHostRedirect(t *testing.T) {
	f := newFileRedirectFixture(t)
	if _, err := f.svc.RequestHTTP(f.id, "alice", "POST", "/to-other", []byte(`{"a":1}`), "", nil); err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if n := len(f.otherRec.all()); n != 1 {
		t.Errorf("the other host received %d requests, want the unchanged followed redirect", n)
	}
}
