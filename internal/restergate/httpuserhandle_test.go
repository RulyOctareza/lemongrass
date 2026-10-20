package restergate

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func spacedUsers() []vault.ResterUser {
	return []vault.ResterUser{
		{Name: "John Doe", Fields: map[string]string{"u": "john"}, Tags: []string{"tenant x"}},
		{Name: "Bob", Fields: map[string]string{"u": "bob"}},
	}
}

func TestRequestHTTPResolvesASpacedUserThroughItsHandle(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, spacedUsers(), []string{"GET"})

	for _, typed := range []string{"john_doe", "John Doe", "JOHN_DOE"} {
		mustRequest(t, svc, id, typed)
	}
	if logins.Load() != 1 {
		t.Errorf("logins = %d, want 1, every spelling reaching the same user and its cached token", logins.Load())
	}
}

func TestFlushHTTPTokensByHandle(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, spacedUsers(), []string{"GET"})

	mustRequest(t, svc, id, "john_doe")
	mustRequest(t, svc, id, "bob")
	flushed, err := svc.FlushHTTPTokens(id, "john_doe")
	if err != nil || flushed != 1 {
		t.Fatalf("FlushHTTPTokens = %d, %v, want 1 evicted", flushed, err)
	}
}

func TestModelSeesHandlesNotStoredNames(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, spacedUsers(), []string{"GET"})

	users, err := svc.HTTPChannelUsers(id)
	if err != nil || len(users) != 2 || users[0].Name != "john_doe" || users[1].Name != "bob" {
		t.Fatalf("HTTPChannelUsers = %+v, %v, want handles john_doe and bob", users, err)
	}
	info, err := svc.HTTPChannelInfo(id)
	if err != nil || info.Users[0].Name != "john_doe" {
		t.Errorf("info users = %+v, %v, want the handle", info.Users, err)
	}

	_, err = svc.RequestHTTP(id, "mallory", "GET", "/api/x", nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "john_doe (tenant x)") || strings.Contains(err.Error(), "John Doe") {
		t.Errorf("err = %v, want a list showing john_doe (tenant x) and no raw name", err)
	}
}
