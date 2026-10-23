package gatekeeper

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

const testRootSecret = "correct horse battery staple"

// unreachableConnString points at a port nothing listens on, on loopback, so a query travels the whole round trip and fails only at execution.
const unreachableConnString = "mysql://root:secret@127.0.0.1:1/appdb"

const editedConnString = "mysql://root:rotated@127.0.0.1:2/appdb"

func fullScope() vault.Scope {
	return vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
}

func startTestServer(t *testing.T) *Client {
	t.Helper()
	svc, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	limiter := vault.NewFailureLimiter(1000, time.Minute, time.Hour)
	go Serve(svc, l, limiter)
	t.Cleanup(func() { l.Close() })
	return &Client{SocketPath: sockPath}
}

func TestIPCFullChannelLifecycle(t *testing.T) {
	client := startTestServer(t)

	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	c, err := client.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateChannel returned an empty channel id")
	}

	scope, err := client.ChannelScope(c.ID)
	if err != nil {
		t.Fatalf("ChannelScope: %v", err)
	}
	if scope.DBName != "app-backend" {
		t.Errorf("ChannelScope.DBName = %q, want app-backend", scope.DBName)
	}

	// Reaches execution and fails only because nothing's listening -- proof the query travelled the whole IPC round trip.
	_, err = client.Query(c.ID, "SELECT id FROM employees")
	if err == nil || !strings.Contains(err.Error(), "executing query") {
		t.Errorf("Query = %v, want it to fail at execution", err)
	}

	if _, err := client.Query(c.ID, "SELECT id FROM salaries"); err == nil {
		t.Error("Query on an out-of-scope table returned nil error")
	}

	if _, err := client.Activate(testRootSecret, c.ID, 10*time.Minute); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	channels, err := client.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != c.ID {
		t.Fatalf("ListChannels() = %v, want only %s", channels, c.ID)
	}

	if err := client.Revoke(c.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := client.Query(c.ID, "SELECT id FROM employees"); err == nil {
		t.Error("Query after Revoke returned nil error")
	}

	channels, err = client.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels after revoke: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("ListChannels after revoke = %v, want none", channels)
	}
}

func TestIPCAdminOpsLockOutAfterRepeatedWrongSecret(t *testing.T) {
	svc, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	limiter := vault.NewFailureLimiter(3, time.Minute, time.Hour)
	go Serve(svc, l, limiter)
	client := &Client{SocketPath: sockPath}

	for i := 0; i < 3; i++ {
		if _, err := client.CreateChannel("wrong passphrase", "test-channel", "app-backend", fullScope(), time.Minute); err == nil {
			t.Fatalf("CreateChannel with a wrong passphrase (attempt %d) returned nil error", i)
		}
	}

	// The limiter should now be locked out even for a correct secret.
	if _, err := client.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), time.Minute); err == nil {
		t.Error("CreateChannel with the correct secret after lockout returned nil error")
	}
}

func TestIPCListTablesRoundTrips(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	// Reaches the listing query and fails only because nothing's listening -- proof it
	// travelled the whole IPC round trip, not a wrong-passphrase or dispatch failure.
	_, err := client.ListTables(testRootSecret, "app-backend")
	if err == nil || !strings.Contains(err.Error(), "listing tables") {
		t.Errorf("ListTables = %v, want it to fail at the listing query", err)
	}

	if _, err := client.ListTables("wrong passphrase", "app-backend"); err == nil {
		t.Error("ListTables with the wrong root secret returned nil error")
	}
}

func TestIPCConnectionSSHHostsRoundTrips(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "direct", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	if err := client.PutCredential(testRootSecret, "tunnelled", []byte("mysql://root:secret@db-internal:3306/appdb?lgrass_ssh=bastion-prod")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	hosts, err := client.ConnectionSSHHosts(testRootSecret)
	if err != nil {
		t.Fatalf("ConnectionSSHHosts: %v", err)
	}
	if len(hosts) != 1 || hosts["tunnelled"] != "bastion-prod" {
		t.Errorf("ConnectionSSHHosts = %v, want only tunnelled via bastion-prod", hosts)
	}

	if _, err := client.ConnectionSSHHosts("wrong passphrase"); err == nil {
		t.Error("ConnectionSSHHosts with the wrong root secret returned nil error")
	}
}

func writeScope() vault.Scope {
	return vault.Scope{Tables: []string{"employees"}, Operations: []string{"delete"}}
}

func TestIPCExecuteRoundTrip(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := client.CreateChannel(testRootSecret, "test-channel", "app-backend", writeScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// Reaches transaction open and fails only because nothing's listening -- proof the write travelled the whole IPC round trip.
	_, err = client.Execute(c.ID, "DELETE FROM employees WHERE id = 1", true, "tester")
	if err == nil || !strings.Contains(err.Error(), "beginning transaction") {
		t.Errorf("Execute = %v, want it to fail opening the transaction", err)
	}

	if _, err := client.Execute(c.ID, "DELETE FROM salaries WHERE id = 1", true, "tester"); err == nil {
		t.Error("Execute on an out-of-scope table returned nil error")
	}

	if err := client.Revoke(c.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := client.Execute(c.ID, "DELETE FROM employees WHERE id = 1", true, "tester"); err == nil {
		t.Error("Execute after Revoke returned nil error")
	}
}

func TestIPCExecuteIsNotAdminGated(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := client.CreateChannel(testRootSecret, "test-channel", "app-backend", writeScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// Many Execute calls in a row should never trip the admin limiter -- it only gates root-secret-bearing ops.
	for i := 0; i < 20; i++ {
		_, err := client.Execute(c.ID, "DELETE FROM employees WHERE id = 1", true, "tester")
		if err == nil || !strings.Contains(err.Error(), "beginning transaction") {
			t.Fatalf("Execute attempt %d = %v, want it to fail opening the transaction, not be admin-gated", i, err)
		}
	}
}

func TestIPCQueryIsNotAdminGated(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := client.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// Many Query calls in a row should never trip the admin limiter -- it only gates root-secret-bearing ops.
	for i := 0; i < 20; i++ {
		_, err := client.Query(c.ID, "SELECT id FROM employees")
		if err == nil || !strings.Contains(err.Error(), "executing query") {
			t.Fatalf("Query attempt %d = %v, want it to fail at execution, not be admin-gated", i, err)
		}
	}
}

func TestIPCGetAndUpdateResterRoundTrip(t *testing.T) {
	client := startTestServer(t)

	d := vaulttest.Domain{BaseURL: "http://a.invalid", Users: []vault.ResterUser{{Name: "alice", Fields: map[string]string{"password": "pw"}, Tags: []string{"tenant x"}}}}
	if err := client.PutRester(testRootSecret, "staging", d.Rester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}

	got, err := client.GetRester(testRootSecret, "staging")
	if err != nil {
		t.Fatalf("GetRester: %v", err)
	}
	if got.Environments[0].BaseURL != "http://a.invalid" || got.Users[0].Fields["password"] != "pw" || got.Users[0].Tags[0] != "tenant x" {
		t.Errorf("GetRester = %+v, want the full stored config", got)
	}

	got.Environments[0].BaseURL = "http://b.invalid"
	if err := client.UpdateRester(testRootSecret, "staging", got); err != nil {
		t.Fatalf("UpdateRester: %v", err)
	}
	again, err := client.GetRester(testRootSecret, "staging")
	if err != nil || again.Environments[0].BaseURL != "http://b.invalid" {
		t.Errorf("after UpdateRester: %+v, err = %v, want base URL http://b.invalid", again, err)
	}

	if _, err := client.GetRester("wrong-secret", "staging"); err == nil {
		t.Error("GetRester accepted a wrong passphrase")
	}
}

func TestIPCGetAndUpdateConnectionRoundTrip(t *testing.T) {
	client := startTestServer(t)
	if err := client.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}

	got, err := client.GetConnection(testRootSecret, "app-backend")
	if err != nil || got != unreachableConnString {
		t.Fatalf("GetConnection = %q, %v", got, err)
	}
	if err := client.UpdateConnection(testRootSecret, "app-backend", editedConnString); err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	again, err := client.GetConnection(testRootSecret, "app-backend")
	if err != nil || again != editedConnString {
		t.Errorf("after UpdateConnection: %q, %v, want the edited string", again, err)
	}
	if _, err := client.GetConnection("wrong-secret", "app-backend"); err == nil {
		t.Error("GetConnection accepted a wrong passphrase")
	}
}
