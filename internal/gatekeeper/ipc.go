// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/faizalv/lemongrass/internal/dbgate"
	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

const (
	opPutCredential       = "put_credential"
	opCreateChannel       = "create_channel"
	opActivate            = "activate"
	opQuery               = "query"
	opExecute             = "execute"
	opRevoke              = "revoke"
	opChannelScope        = "channel_scope"
	opListChannels        = "list_channels"
	opListConnections     = "list_connections"
	opDeleteConnection    = "delete_connection"
	opTestConnection      = "test_connection"
	opTestConnectionSaved = "test_connection_saved"
	opListTables          = "list_tables"
	opConnectionSSHHosts  = "connection_ssh_hosts"
	opHasPassphrase       = "has_passphrase"
	opSetPassphrase       = "set_passphrase"
	opVerifyPassphrase    = "verify_passphrase"
	opResetVault          = "reset_vault"
	opGetConnection       = "get_connection"
	opUpdateConnection    = "update_connection"

	opPutRester           = "put_rester"
	opListResters         = "list_resters"
	opDeleteRester        = "delete_rester"
	opTestResterLogin     = "test_rester_login"
	opCreateHTTPChannel   = "create_http_channel"
	opActivateHTTP        = "activate_http"
	opRequestHTTP         = "request_http"
	opRequestHTTPDownload = "request_http_download"
	opRevokeHTTP          = "revoke_http"
	opHTTPChannelScope    = "http_channel_scope"
	opListHTTPChannels    = "list_http_channels"
	opHTTPChannelInfo     = "http_channel_info"
	opHTTPChannelUsers    = "http_channel_users"
	opFlushHTTPTokens     = "flush_http_tokens"
	opGetRester           = "get_rester"
	opUpdateRester        = "update_rester"
	opWebLoginFields      = "web_login_fields"
	opAppLogin            = "app_login"
)

var connDeadline = 10 * time.Second

type request struct {
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type response struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type putCredentialPayload struct {
	RootSecret string `json:"root_secret"`
	DBName     string `json:"db_name"`
	Value      []byte `json:"value"`
}

type createChannelPayload struct {
	RootSecret string      `json:"root_secret"`
	Name       string      `json:"name"`
	DBName     string      `json:"db_name"`
	Scope      vault.Scope `json:"scope"`
	TTLSeconds int         `json:"ttl_seconds"`
}

type activatePayload struct {
	RootSecret string          `json:"root_secret"`
	ID         vault.ChannelID `json:"id"`
	TTLSeconds int             `json:"ttl_seconds"`
}

type queryPayload struct {
	ID  vault.ChannelID `json:"id"`
	SQL string          `json:"sql"`
}

type queryResultPayload struct {
	Result dbgate.QueryResult `json:"result"`
}

type executePayload struct {
	ID     vault.ChannelID `json:"id"`
	SQL    string          `json:"sql"`
	Commit bool            `json:"commit"`
	// Actor is the caller-facing identity Execute audits under.
	Actor string `json:"actor,omitempty"`
}

type executeResultPayload struct {
	Result dbgate.WriteResult `json:"result"`
}

type channelIDPayload struct {
	ID vault.ChannelID `json:"id"`
}

type channelPayload struct {
	Channel vault.Channel `json:"channel"`
}

type channelListPayload struct {
	Channels []vault.Channel `json:"channels"`
}

type connectionListPayload struct {
	Names []string `json:"names"`
}

type dbNamePayload struct {
	DBName string `json:"db_name"`
}

type testConnectionPayload struct {
	ConnectionString string `json:"connection_string"`
}

type testConnectionSavedPayload struct {
	RootSecret string `json:"root_secret"`
	DBName     string `json:"db_name"`
}

type connectionStringPayload struct {
	ConnectionString string `json:"connection_string"`
}

type updateConnectionPayload struct {
	RootSecret       string `json:"root_secret"`
	DBName           string `json:"db_name"`
	ConnectionString string `json:"connection_string"`
}

type rootSecretPayload struct {
	RootSecret string `json:"root_secret"`
}

type sshHostsPayload struct {
	Hosts map[string]string `json:"hosts"`
}

type tablesPayload struct {
	Tables []string `json:"tables"`
}

type hasPassphrasePayload struct {
	HasPassphrase bool `json:"has_passphrase"`
}

type putResterPayload struct {
	RootSecret string       `json:"root_secret"`
	Name       string       `json:"name"`
	Rester     vault.Rester `json:"rester"`
}

type resterNamePayload struct {
	Name string `json:"name"`
}

type resterListPayload struct {
	Names []string `json:"names"`
}

type testResterLoginPayload struct {
	API         vault.ResterAPI   `json:"api"`
	Environment vault.Environment `json:"environment"`
	User        vault.ResterUser  `json:"user"`
}

// testResterLoginResultPayload is a finished login test. An empty Error means the login passed;
// Status and Body are set only when the login endpoint answered with a non-2xx status.
type testResterLoginResultPayload struct {
	Error  string `json:"error,omitempty"`
	Status int    `json:"status,omitempty"`
	Body   string `json:"body,omitempty"`
}

func testResterLoginResult(err error) testResterLoginResultPayload {
	if err == nil {
		return testResterLoginResultPayload{}
	}
	result := testResterLoginResultPayload{Error: err.Error()}
	var statusErr *restergate.LoginStatusError
	if errors.As(err, &statusErr) {
		result.Status = statusErr.Status
		result.Body = string(statusErr.Body)
	}
	return result
}

type createHTTPChannelPayload struct {
	RootSecret  string          `json:"root_secret"`
	Name        string          `json:"name"`
	Rester      string          `json:"rester"`
	Environment string          `json:"environment"`
	Type        string          `json:"type"`
	Scope       vault.HTTPScope `json:"scope"`
	WebScope    *vault.WebScope `json:"web_scope"`
	TTLSeconds  int             `json:"ttl_seconds"`
}

type activateHTTPPayload struct {
	RootSecret string          `json:"root_secret"`
	ID         vault.ChannelID `json:"id"`
	TTLSeconds int             `json:"ttl_seconds"`
}

type requestHTTPPayload struct {
	ID          vault.ChannelID `json:"id"`
	User        string          `json:"user"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Body        []byte          `json:"body"`
	ContentType string          `json:"content_type"`
	Headers     []vault.Header  `json:"headers,omitempty"`
	// Actor is the caller-facing identity RequestHTTP will audit under once the audit write
	// lands; carried over the wire already so that build doesn't need another round of
	// payload changes.
	Actor string `json:"actor,omitempty"`
}

type httpResultPayload struct {
	Result restergate.HTTPResult `json:"result"`
}

type httpChannelPayload struct {
	Channel vault.HTTPChannel `json:"channel"`
}

type httpChannelInfoPayload struct {
	Info restergate.HTTPChannelInfo `json:"info"`
}

type httpChannelUsersPayload struct {
	Users []restergate.HTTPUserInfo `json:"users"`
}

type flushHTTPTokensPayload struct {
	ID   vault.ChannelID `json:"id"`
	User string          `json:"user"`
}

type webLoginFieldsPayload struct {
	ChannelID vault.ChannelID `json:"channel_id"`
	User      string          `json:"user"`
}

type appLoginPayload struct {
	ID     vault.ChannelID `json:"id"`
	User   string          `json:"user"`
	Serial string          `json:"serial,omitempty"`
}

type flushedPayload struct {
	Flushed int `json:"flushed"`
}

type getResterPayload struct {
	RootSecret string `json:"root_secret"`
	Name       string `json:"name"`
}

type resterPayload struct {
	Rester vault.Rester `json:"rester"`
}

type httpChannelListPayload struct {
	Channels []vault.HTTPChannel `json:"channels"`
}

// Serve accepts connections on l and handles one request per connection.
// adminLimiter gates PutCredential, CreateChannel, and Activate -- the
// operations a request carries a root secret for.
func Serve(svc *Backend, l net.Listener, adminLimiter *vault.FailureLimiter) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go handleConn(svc, adminLimiter, conn)
	}
}

func handleConn(svc *Backend, adminLimiter *vault.FailureLimiter, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(connDeadline))

	if err := vault.VerifyPeerUID(conn); err != nil {
		json.NewEncoder(conn).Encode(errResponse(err))
		return
	}

	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(errResponse(err))
		return
	}
	if requiresPeerBinaryCheck(req.Op) {
		if err := vault.VerifyPeerBinary(conn); err != nil {
			json.NewEncoder(conn).Encode(errResponse(err))
			return
		}
	}
	if req.Op == opRequestHTTP || req.Op == opRequestHTTPDownload {
		conn.SetDeadline(time.Now().Add(restergate.RequestTimeout))
	}
	if req.Op == opAppLogin {
		conn.SetDeadline(time.Now().Add(restergate.AppLoginTimeout))
	}
	if req.Op == opRequestHTTPDownload {
		serveDownload(svc, conn, req)
		return
	}
	json.NewEncoder(conn).Encode(dispatch(svc, adminLimiter, req))
}

// requiresPeerBinaryCheck reports whether op is one the agent process (the lgrass binary)
// issues, as opposed to Electron's admin ops -- Electron's own binary path isn't fixed yet,
// so those ops stay at UID-only verification.
func requiresPeerBinaryCheck(op string) bool {
	return op == opCurrentPolicy || op == opQuery || op == opExecute || op == opChannelScope || op == opRequestHTTP || op == opRequestHTTPDownload || op == opHTTPChannelScope || op == opHTTPChannelInfo || op == opHTTPChannelUsers || op == opFlushHTTPTokens || op == opAppLogin
}

func dispatch(svc *Backend, adminLimiter *vault.FailureLimiter, req request) response {
	switch req.Op {
	case opPutCredential:
		var p putCredentialPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.PutCredential(p.RootSecret, p.DBName, p.Value)
		})

	case opCreateChannel:
		var p createChannelPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			c, err := svc.CreateChannel(p.RootSecret, p.Name, p.DBName, p.Scope, time.Duration(p.TTLSeconds)*time.Second)
			return payloadResponse(channelPayload{Channel: c}), err
		})

	case opActivate:
		var p activatePayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			c, err := svc.Activate(p.RootSecret, p.ID, time.Duration(p.TTLSeconds)*time.Second)
			return payloadResponse(channelPayload{Channel: c}), err
		})

	case opQuery:
		var p queryPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		result, err := svc.DB.Query(p.ID, p.SQL)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(queryResultPayload{Result: result})

	case opExecute:
		var p executePayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		result, err := svc.DB.Execute(p.ID, p.SQL, p.Commit, p.Actor)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(executeResultPayload{Result: result})

	case opRevoke:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		if err := svc.Revoke(p.ID); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opChannelScope:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		c, err := svc.ChannelScope(p.ID)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(channelPayload{Channel: c})

	case opListChannels:
		channels, err := svc.ListChannels()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(channelListPayload{Channels: channels})

	case opListConnections:
		names, err := svc.ListConnections()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(connectionListPayload{Names: names})

	case opDeleteConnection:
		var p dbNamePayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		if err := svc.DeleteConnection(p.DBName); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opTestConnection:
		var p testConnectionPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		if err := svc.DB.TestConnection(p.ConnectionString); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opTestConnectionSaved:
		var p testConnectionSavedPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.DB.TestConnectionSaved(p.RootSecret, p.DBName)
		})

	case opGetConnection:
		var p testConnectionSavedPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			connString, err := svc.GetConnection(p.RootSecret, p.DBName)
			return payloadResponse(connectionStringPayload{ConnectionString: connString}), err
		})

	case opUpdateConnection:
		var p updateConnectionPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.DB.UpdateConnection(p.RootSecret, p.DBName, p.ConnectionString)
		})

	case opListTables:
		var p testConnectionSavedPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			tables, err := svc.DB.ListTables(p.RootSecret, p.DBName)
			return payloadResponse(tablesPayload{Tables: tables}), err
		})

	case opConnectionSSHHosts:
		var p rootSecretPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			hosts, err := svc.DB.ConnectionSSHHosts(p.RootSecret)
			return payloadResponse(sshHostsPayload{Hosts: hosts}), err
		})

	case opHasPassphrase:
		has, err := svc.HasPassphrase()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(hasPassphrasePayload{HasPassphrase: has})

	case opSetPassphrase:
		var p rootSecretPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.SetPassphrase(p.RootSecret)
		})

	case opVerifyPassphrase:
		var p rootSecretPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.VerifyPassphrase(p.RootSecret)
		})

	case opResetVault:
		if err := svc.ResetVault(); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opPutRester:
		var p putResterPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.PutRester(p.RootSecret, p.Name, p.Rester)
		})

	case opListResters:
		names, err := svc.ListResters()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(resterListPayload{Names: names})

	case opDeleteRester:
		var p resterNamePayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		if err := svc.DeleteRester(p.Name); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opTestResterLogin:
		var p testResterLoginPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return payloadResponse(testResterLoginResult(svc.HTTP.TestResterLogin(p.API, p.Environment, p.User)))

	case opCreateHTTPChannel:
		var p createHTTPChannelPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			c, err := svc.CreateHTTPChannel(p.RootSecret, p.Name, p.Rester, p.Environment, p.Type, p.Scope, p.WebScope, time.Duration(p.TTLSeconds)*time.Second)
			return payloadResponse(httpChannelPayload{Channel: c}), err
		})

	case opActivateHTTP:
		var p activateHTTPPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			c, err := svc.ActivateHTTP(p.RootSecret, p.ID, time.Duration(p.TTLSeconds)*time.Second)
			return payloadResponse(httpChannelPayload{Channel: c}), err
		})

	case opRequestHTTP:
		var p requestHTTPPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		result, err := svc.HTTP.RequestHTTP(p.ID, p.User, p.Method, p.Path, p.Body, p.ContentType, p.Headers)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(httpResultPayload{Result: result})

	case opRevokeHTTP:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		if err := svc.RevokeHTTP(p.ID); err != nil {
			return errResponse(err)
		}
		return response{OK: true}

	case opHTTPChannelScope:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		c, err := svc.HTTPChannelScope(p.ID)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(httpChannelPayload{Channel: c})

	case opHTTPChannelInfo:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		info, err := svc.HTTP.HTTPChannelInfo(p.ID)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(httpChannelInfoPayload{Info: info})

	case opHTTPChannelUsers:
		var p channelIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		users, err := svc.HTTP.HTTPChannelUsers(p.ID)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(httpChannelUsersPayload{Users: users})

	case opWebLoginFields:
		var p webLoginFieldsPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		fields, err := svc.HTTP.WebLoginFields(p.ChannelID, p.User)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(fields)

	case opAppLogin:
		var p appLoginPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		result, err := svc.HTTP.AppLogin(p.ID, p.User, p.Serial)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(result)

	case opFlushHTTPTokens:
		var p flushHTTPTokensPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		flushed, err := svc.HTTP.FlushHTTPTokens(p.ID, p.User)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(flushedPayload{Flushed: flushed})

	case opGetRester:
		var p getResterPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			d, err := svc.GetRester(p.RootSecret, p.Name)
			return payloadResponse(resterPayload{Rester: d}), err
		})

	case opUpdateRester:
		var p putResterPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.UpdateRester(p.RootSecret, p.Name, p.Rester)
		})

	case opListHTTPChannels:
		channels, err := svc.ListHTTPChannels()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(httpChannelListPayload{Channels: channels})

	case opPutPolicy:
		var p putPolicyPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.PutPolicy(p.RootSecret, p.ProjectID, p.Policy)
		})

	case opMigratePolicy:
		var p rootSecretPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return adminOp(adminLimiter, func() (response, error) {
			return response{OK: true}, svc.MigrateSealedPolicy(p.RootSecret)
		})

	case opListPolicies:
		global, projects := svc.Policies()
		out := policiesPayload{Global: global, Projects: make(map[string]json.RawMessage, len(projects))}
		for id, p := range projects {
			out.Projects[id] = p
		}
		return payloadResponse(out)

	case opCurrentPolicy:
		var p currentPolicyPayload
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &p); err != nil {
				return errResponse(err)
			}
		}
		raw, err := svc.EffectivePolicy(p.ProjectID).Marshal()
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(policyPayload{Policy: raw})

	case opPolicyCatalog:
		return payloadResponse(policyCatalogPayload{Rules: guard.Catalog()})

	default:
		return errResponse(fmt.Errorf("gatekeeper: unknown op %q", req.Op))
	}
}

// adminOp checks the limiter before a root-secret-bearing call and counts only wrong-secret failures, so repeated guesses lock out while an unreachable database or a bad input does not.
func adminOp(adminLimiter *vault.FailureLimiter, fn func() (response, error)) response {
	if err := adminLimiter.Check(); err != nil {
		return errResponse(err)
	}
	resp, err := fn()
	if err != nil {
		if vault.IsAuthFailure(err) {
			adminLimiter.RecordFailure()
		}
		return errResponse(err)
	}
	adminLimiter.RecordSuccess()
	return resp
}

func errResponse(err error) response {
	return response{OK: false, Error: err.Error()}
}

func payloadResponse(v interface{}) response {
	b, err := json.Marshal(v)
	if err != nil {
		return errResponse(err)
	}
	return response{OK: true, Payload: b}
}

// Client talks to a running vault daemon over its unix socket.
type Client struct {
	SocketPath string
	Timeout    time.Duration
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return connDeadline
}

func (c *Client) call(op string, payload, out interface{}) error {
	return c.callWithin(c.timeout(), op, payload, out)
}

func (c *Client) callWithin(timeout time.Duration, op string, payload, out interface{}) error {
	conn, err := net.DialTimeout("unix", c.SocketPath, timeout)
	if err != nil {
		return fmt.Errorf("gatekeeper: dialing %s: %w", c.SocketPath, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	var payloadBytes json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		payloadBytes = b
	}
	if err := json.NewEncoder(conn).Encode(request{Op: op, Payload: payloadBytes}); err != nil {
		return fmt.Errorf("gatekeeper: sending request: %w", err)
	}

	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("gatekeeper: reading response: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out != nil && resp.Payload != nil {
		return json.Unmarshal(resp.Payload, out)
	}
	return nil
}

func (c *Client) PutCredential(rootSecret, dbName string, value []byte) error {
	return c.call(opPutCredential, putCredentialPayload{RootSecret: rootSecret, DBName: dbName, Value: value}, nil)
}

func (c *Client) CreateChannel(rootSecret, name, dbName string, scope vault.Scope, ttl time.Duration) (vault.Channel, error) {
	var out channelPayload
	err := c.call(opCreateChannel, createChannelPayload{RootSecret: rootSecret, Name: name, DBName: dbName, Scope: scope, TTLSeconds: int(ttl.Seconds())}, &out)
	return out.Channel, err
}

func (c *Client) Activate(rootSecret string, id vault.ChannelID, ttl time.Duration) (vault.Channel, error) {
	var out channelPayload
	err := c.call(opActivate, activatePayload{RootSecret: rootSecret, ID: id, TTLSeconds: int(ttl.Seconds())}, &out)
	return out.Channel, err
}

func (c *Client) Query(id vault.ChannelID, sqlText string) (dbgate.QueryResult, error) {
	var out queryResultPayload
	err := c.call(opQuery, queryPayload{ID: id, SQL: sqlText}, &out)
	return out.Result, err
}

func (c *Client) Execute(id vault.ChannelID, sqlText string, commit bool, actor string) (dbgate.WriteResult, error) {
	var out executeResultPayload
	err := c.call(opExecute, executePayload{ID: id, SQL: sqlText, Commit: commit, Actor: actor}, &out)
	return out.Result, err
}

func (c *Client) Revoke(id vault.ChannelID) error {
	return c.call(opRevoke, channelIDPayload{ID: id}, nil)
}

func (c *Client) ChannelScope(id vault.ChannelID) (vault.Channel, error) {
	var out channelPayload
	err := c.call(opChannelScope, channelIDPayload{ID: id}, &out)
	return out.Channel, err
}

func (c *Client) ListChannels() ([]vault.Channel, error) {
	var out channelListPayload
	err := c.call(opListChannels, nil, &out)
	return out.Channels, err
}

func (c *Client) ListConnections() ([]string, error) {
	var out connectionListPayload
	err := c.call(opListConnections, nil, &out)
	return out.Names, err
}

func (c *Client) DeleteConnection(dbName string) error {
	return c.call(opDeleteConnection, dbNamePayload{DBName: dbName}, nil)
}

func (c *Client) TestConnection(connectionString string) error {
	return c.call(opTestConnection, testConnectionPayload{ConnectionString: connectionString}, nil)
}

func (c *Client) TestConnectionSaved(rootSecret, dbName string) error {
	return c.call(opTestConnectionSaved, testConnectionSavedPayload{RootSecret: rootSecret, DBName: dbName}, nil)
}

func (c *Client) GetConnection(rootSecret, dbName string) (string, error) {
	var out connectionStringPayload
	err := c.call(opGetConnection, testConnectionSavedPayload{RootSecret: rootSecret, DBName: dbName}, &out)
	return out.ConnectionString, err
}

func (c *Client) UpdateConnection(rootSecret, dbName, connectionString string) error {
	return c.call(opUpdateConnection, updateConnectionPayload{RootSecret: rootSecret, DBName: dbName, ConnectionString: connectionString}, nil)
}

func (c *Client) ListTables(rootSecret, dbName string) ([]string, error) {
	var out tablesPayload
	err := c.call(opListTables, testConnectionSavedPayload{RootSecret: rootSecret, DBName: dbName}, &out)
	return out.Tables, err
}

func (c *Client) ConnectionSSHHosts(rootSecret string) (map[string]string, error) {
	var out sshHostsPayload
	err := c.call(opConnectionSSHHosts, rootSecretPayload{RootSecret: rootSecret}, &out)
	return out.Hosts, err
}

func (c *Client) HasPassphrase() (bool, error) {
	var out hasPassphrasePayload
	err := c.call(opHasPassphrase, nil, &out)
	return out.HasPassphrase, err
}

func (c *Client) SetPassphrase(rootSecret string) error {
	return c.call(opSetPassphrase, rootSecretPayload{RootSecret: rootSecret}, nil)
}

func (c *Client) VerifyPassphrase(rootSecret string) error {
	return c.call(opVerifyPassphrase, rootSecretPayload{RootSecret: rootSecret}, nil)
}

func (c *Client) ResetVault() error {
	return c.call(opResetVault, nil, nil)
}

func (c *Client) PutRester(rootSecret, name string, r vault.Rester) error {
	return c.call(opPutRester, putResterPayload{RootSecret: rootSecret, Name: name, Rester: r}, nil)
}

func (c *Client) ListResters() ([]string, error) {
	var out resterListPayload
	err := c.call(opListResters, nil, &out)
	return out.Names, err
}

func (c *Client) DeleteRester(name string) error {
	return c.call(opDeleteRester, resterNamePayload{Name: name}, nil)
}

func (c *Client) TestResterLogin(api vault.ResterAPI, env vault.Environment, u vault.ResterUser) error {
	var out testResterLoginResultPayload
	if err := c.call(opTestResterLogin, testResterLoginPayload{API: api, Environment: env, User: u}, &out); err != nil {
		return err
	}
	if out.Error != "" {
		return errors.New(out.Error)
	}
	return nil
}

func (c *Client) CreateHTTPChannel(rootSecret, name, rester, environment, typ string, scope vault.HTTPScope, webScope *vault.WebScope, ttl time.Duration) (vault.HTTPChannel, error) {
	var out httpChannelPayload
	err := c.call(opCreateHTTPChannel, createHTTPChannelPayload{RootSecret: rootSecret, Name: name, Rester: rester, Environment: environment, Type: typ, Scope: scope, WebScope: webScope, TTLSeconds: int(ttl.Seconds())}, &out)
	return out.Channel, err
}

func (c *Client) ActivateHTTP(rootSecret string, id vault.ChannelID, ttl time.Duration) (vault.HTTPChannel, error) {
	var out httpChannelPayload
	err := c.call(opActivateHTTP, activateHTTPPayload{RootSecret: rootSecret, ID: id, TTLSeconds: int(ttl.Seconds())}, &out)
	return out.Channel, err
}

func (c *Client) RequestHTTP(id vault.ChannelID, user, method, path string, body []byte, contentType string, headers []vault.Header, actor string) (restergate.HTTPResult, error) {
	var out httpResultPayload
	err := c.callWithin(max(c.Timeout, restergate.RequestTimeout), opRequestHTTP, requestHTTPPayload{ID: id, User: user, Method: method, Path: path, Body: body, ContentType: contentType, Headers: headers, Actor: actor}, &out)
	return out.Result, err
}

func (c *Client) RevokeHTTP(id vault.ChannelID) error {
	return c.call(opRevokeHTTP, channelIDPayload{ID: id}, nil)
}

func (c *Client) HTTPChannelScope(id vault.ChannelID) (vault.HTTPChannel, error) {
	var out httpChannelPayload
	err := c.call(opHTTPChannelScope, channelIDPayload{ID: id}, &out)
	return out.Channel, err
}

func (c *Client) HTTPChannelInfo(id vault.ChannelID) (restergate.HTTPChannelInfo, error) {
	var out httpChannelInfoPayload
	err := c.call(opHTTPChannelInfo, channelIDPayload{ID: id}, &out)
	return out.Info, err
}

func (c *Client) HTTPChannelUsers(id vault.ChannelID) ([]restergate.HTTPUserInfo, error) {
	var out httpChannelUsersPayload
	err := c.call(opHTTPChannelUsers, channelIDPayload{ID: id}, &out)
	return out.Users, err
}

// WebLoginFields asks for the login material of one user on a web channel. Only the app calls it; its reply never travels to a model.
func (c *Client) WebLoginFields(id vault.ChannelID, user string) (restergate.WebLoginFields, error) {
	var out restergate.WebLoginFields
	err := c.call(opWebLoginFields, webLoginFieldsPayload{ChannelID: id, User: user}, &out)
	return out, err
}

// AppLogin asks the vault to log the channel's app in on the connected phone as user. The reply names the user and the outcome, never a field value.
func (c *Client) AppLogin(id vault.ChannelID, user, serial string) (restergate.AppLoginResult, error) {
	var out restergate.AppLoginResult
	err := c.callWithin(max(c.Timeout, restergate.AppLoginTimeout), opAppLogin, appLoginPayload{ID: id, User: user, Serial: serial}, &out)
	return out, err
}

func (c *Client) FlushHTTPTokens(id vault.ChannelID, user string) (int, error) {
	var out flushedPayload
	err := c.call(opFlushHTTPTokens, flushHTTPTokensPayload{ID: id, User: user}, &out)
	return out.Flushed, err
}

func (c *Client) GetRester(rootSecret, name string) (vault.Rester, error) {
	var out resterPayload
	err := c.call(opGetRester, getResterPayload{RootSecret: rootSecret, Name: name}, &out)
	return out.Rester, err
}

func (c *Client) UpdateRester(rootSecret, name string, r vault.Rester) error {
	return c.call(opUpdateRester, putResterPayload{RootSecret: rootSecret, Name: name, Rester: r}, nil)
}

func (c *Client) ListHTTPChannels() ([]vault.HTTPChannel, error) {
	var out httpChannelListPayload
	err := c.call(opListHTTPChannels, nil, &out)
	return out.Channels, err
}
