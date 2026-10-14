package restergate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func newCountingLoginServer(logins *atomic.Int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		n := logins.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "n": n})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return httptest.NewServer(mux)
}

func twoLoginUsers() []vault.DomainUser {
	return []vault.DomainUser{
		{Name: "alice", Fields: map[string]string{"u": "alice"}},
		{Name: "bob", Fields: map[string]string{"u": "bob"}},
	}
}

func mustRequest(t *testing.T, svc *testSvc, id vault.ChannelID, user string) {
	t.Helper()
	if _, err := svc.RequestHTTP(id, user, "GET", "/api/x", nil, "", nil); err != nil {
		t.Fatalf("RequestHTTP as %q: %v", user, err)
	}
}

func TestFlushHTTPTokensAllUsersForcesFreshLogins(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})

	mustRequest(t, svc, id, "alice")
	mustRequest(t, svc, id, "bob")
	mustRequest(t, svc, id, "alice")
	if logins.Load() != 2 {
		t.Fatalf("logins = %d, want 2, one per user and the third request served from cache", logins.Load())
	}

	flushed, err := svc.FlushHTTPTokens(id, "")
	if err != nil || flushed != 2 {
		t.Fatalf("FlushHTTPTokens = %d, %v, want 2 evicted", flushed, err)
	}
	mustRequest(t, svc, id, "alice")
	mustRequest(t, svc, id, "bob")
	if logins.Load() != 4 {
		t.Errorf("logins = %d, want 4, both users logging in again after the flush", logins.Load())
	}
}

func TestFlushHTTPTokensOneUserLeavesOthersCached(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})

	mustRequest(t, svc, id, "alice")
	mustRequest(t, svc, id, "bob")

	flushed, err := svc.FlushHTTPTokens(id, "alice")
	if err != nil || flushed != 1 {
		t.Fatalf("FlushHTTPTokens = %d, %v, want 1 evicted", flushed, err)
	}
	mustRequest(t, svc, id, "alice")
	mustRequest(t, svc, id, "bob")
	if logins.Load() != 3 {
		t.Errorf("logins = %d, want 3, only alice logging in again", logins.Load())
	}
}

func TestFlushHTTPTokensNothingCachedIsZero(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})

	flushed, err := svc.FlushHTTPTokens(id, "")
	if err != nil || flushed != 0 {
		t.Errorf("FlushHTTPTokens = %d, %v, want 0 and no error", flushed, err)
	}
	flushed, err = svc.FlushHTTPTokens(id, "alice")
	if err != nil || flushed != 0 {
		t.Errorf("FlushHTTPTokens for a user with no cached token = %d, %v, want 0 and no error", flushed, err)
	}
}

func TestFlushHTTPTokensUnknownUserListsAvailable(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})

	_, err := svc.FlushHTTPTokens(id, "mallory")
	if err == nil || !strings.Contains(err.Error(), "alice") || !strings.Contains(err.Error(), "bob") {
		t.Errorf("err = %v, want one listing the available users", err)
	}
}

func TestFlushHTTPTokensOnlyTouchesItsOwnChannel(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, first := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})
	second, err := svc.CreateHTTPChannel("root-secret", "second", "test-domain", vault.HTTPScope{Methods: []string{"GET"}}, 3600e9)
	if err != nil {
		t.Fatal(err)
	}

	mustRequest(t, svc, first, "alice")
	mustRequest(t, svc, second.ID, "alice")
	if _, err := svc.FlushHTTPTokens(first, ""); err != nil {
		t.Fatal(err)
	}
	mustRequest(t, svc, second.ID, "alice")
	if logins.Load() != 2 {
		t.Errorf("logins = %d, want 2, the other channel's token still cached", logins.Load())
	}
}

func TestFlushHTTPTokensRequiresLiveChannel(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, twoLoginUsers(), []string{"GET"})

	svc.forgetKey(id)
	if _, err := svc.FlushHTTPTokens(id, ""); err == nil {
		t.Error("expected an error flushing an inactive channel")
	}
	if _, err := svc.FlushHTTPTokens("nosuchchannel", ""); err == nil {
		t.Error("expected an error flushing an unknown channel")
	}
}

func TestFlushHTTPTokensBringYourOwnTokenComesBack(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "carol", Token: "pasted"}}, []string{"GET"})

	mustRequest(t, svc, id, "carol")
	flushed, err := svc.FlushHTTPTokens(id, "carol")
	if err != nil || flushed != 1 {
		t.Fatalf("FlushHTTPTokens = %d, %v, want 1 evicted", flushed, err)
	}
	mustRequest(t, svc, id, "carol")
	if logins.Load() != 0 {
		t.Errorf("logins = %d, want 0, a pasted token never logs in", logins.Load())
	}
}
