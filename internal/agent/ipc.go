// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/faizalv/lemongrass/internal/dbgate"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/web"
)

const (
	opRegisterChannel     = "register_channel"
	opQuery               = "query"
	opExecute             = "execute"
	opRequestHTTP         = "request_http"
	opRequestHTTPDownload = "request_http_download"
	opHTTPChannelInfo     = "http_channel_info"
	opHTTPUsers           = "http_users"
	opHTTPFlush           = "http_flush"
	opAppLogin            = "app_login"
	opForget              = "forget"
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

type registerChannelPayload struct {
	RealID vault.ChannelID `json:"real_id"`
}

type shortIDPayload struct {
	ShortID string `json:"short_id"`
}

type queryPayload struct {
	ShortID string `json:"short_id"`
	SQL     string `json:"sql"`
}

type queryResultPayload struct {
	Result dbgate.QueryResult `json:"result"`
}

type executePayload struct {
	ShortID string `json:"short_id"`
	SQL     string `json:"sql"`
	Commit  bool   `json:"commit"`
	// Actor is the caller-facing identity Execute audits under.
	Actor string `json:"actor,omitempty"`
}

type executeResultPayload struct {
	Result dbgate.WriteResult `json:"result"`
}

type requestHTTPPayload struct {
	ShortID     string         `json:"short_id"`
	User        string         `json:"user"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Body        []byte         `json:"body"`
	ContentType string         `json:"content_type"`
	Headers     []vault.Header `json:"headers,omitempty"`
	// Actor is the caller-facing identity RequestHTTP audits under; empty for RequestHTTPDownload, which is not audited.
	Actor string `json:"actor,omitempty"`
}

type httpResultPayload struct {
	Result restergate.HTTPResult `json:"result"`
}

type httpChannelInfoPayload struct {
	Info restergate.HTTPChannelInfo `json:"info"`
}

type httpUsersPayload struct {
	Users []restergate.HTTPUserInfo `json:"users"`
}

type httpFlushPayload struct {
	ShortID string `json:"short_id"`
	User    string `json:"user"`
}

type appLoginPayload struct {
	ShortID string `json:"short_id"`
	User    string `json:"user"`
	Serial  string `json:"serial,omitempty"`
}

type httpFlushedPayload struct {
	Flushed int `json:"flushed"`
}

// Serve accepts connections on l and handles one request per connection, with queryLimiter gating repeated wrong short-id guesses at Query.
func Serve(svc *Service, l net.Listener, queryLimiter *vault.FailureLimiter) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go handleConn(svc, queryLimiter, conn)
	}
}

func handleConn(svc *Service, queryLimiter *vault.FailureLimiter, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(connDeadline))

	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(errResponse(err))
		return
	}
	if req.Op == opRequestHTTP || req.Op == opRequestHTTPDownload {
		conn.SetDeadline(time.Now().Add(restergate.RequestTimeout))
	}
	if req.Op == opAppLogin {
		conn.SetDeadline(time.Now().Add(restergate.AppLoginTimeout))
	}
	if req.Op == opWebCommand {
		conn.SetDeadline(time.Now().Add(web.Timeout))
	}
	if req.Op == opRequestHTTPDownload {
		serveDownload(svc, conn, req)
		return
	}
	json.NewEncoder(conn).Encode(dispatch(svc, queryLimiter, req))
}

func dispatch(svc *Service, queryLimiter *vault.FailureLimiter, req request) response {
	switch req.Op {
	case opRegisterChannel:
		var p registerChannelPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		shortID, err := svc.RegisterChannel(p.RealID)
		if err != nil {
			return errResponse(err)
		}
		return payloadResponse(shortIDPayload{ShortID: shortID})

	case opQuery:
		var p queryPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			result, err := svc.Query(p.ShortID, p.SQL)
			return payloadResponse(queryResultPayload{Result: result}), err
		})

	case opExecute:
		var p executePayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			result, err := svc.Execute(p.ShortID, p.SQL, p.Commit, p.Actor)
			return payloadResponse(executeResultPayload{Result: result}), err
		})

	case opRequestHTTP:
		var p requestHTTPPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			result, err := svc.RequestHTTP(p.ShortID, p.User, p.Method, p.Path, p.Body, p.ContentType, p.Headers, p.Actor)
			return payloadResponse(httpResultPayload{Result: result}), err
		})

	case opHTTPChannelInfo:
		var p shortIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			info, err := svc.HTTPChannelInfo(p.ShortID)
			return payloadResponse(httpChannelInfoPayload{Info: info}), err
		})

	case opHTTPUsers:
		var p shortIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			users, err := svc.HTTPUsers(p.ShortID)
			return payloadResponse(httpUsersPayload{Users: users}), err
		})

	case opHTTPFlush:
		var p httpFlushPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			flushed, err := svc.FlushHTTPTokens(p.ShortID, p.User)
			return payloadResponse(httpFlushedPayload{Flushed: flushed}), err
		})

	case opAppLogin:
		var p appLoginPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		return queryOp(queryLimiter, func() (response, error) {
			result, err := svc.AppLogin(p.ShortID, p.User, p.Serial)
			return payloadResponse(result), err
		})

	case opWebCommand:
		return webCommandOp(svc, queryLimiter, req.Payload)

	case opForget:
		var p shortIDPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse(err)
		}
		svc.Forget(p.ShortID)
		return response{OK: true}

	default:
		return errResponse(fmt.Errorf("agent: unknown op %q", req.Op))
	}
}

// queryOp checks the limiter before a short-id lookup and counts only unknown short ids as failures, so repeated wrong guesses lock out while rejected or failing queries do not.
func queryOp(queryLimiter *vault.FailureLimiter, fn func() (response, error)) response {
	if err := queryLimiter.Check(); err != nil {
		return errResponse(err)
	}
	resp, err := fn()
	if errors.Is(err, ErrNoSuchChannel) {
		queryLimiter.RecordFailure()
		return errResponse(err)
	}
	queryLimiter.RecordSuccess()
	if err != nil {
		return errResponse(err)
	}
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

func serveDownload(svc *Service, conn net.Conn, req request) {
	var p requestHTTPPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		gatekeeper.WriteDownloadError(conn, err)
		return
	}
	res, err := svc.RequestHTTPDownload(p.ShortID, p.User, p.Method, p.Path, p.Body, p.ContentType, p.Headers)
	if err != nil {
		gatekeeper.WriteDownloadError(conn, err)
		return
	}
	realID, _ := svc.lookup(p.ShortID)
	gatekeeper.WriteDownload(conn, res, restergate.RequestTimeout, func(err error) error { return redactID(err, realID, p.ShortID) })
}

// Client talks to a running agent daemon over its unix socket.
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
		return fmt.Errorf("agent: dialing %s: %w", c.SocketPath, err)
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
		return fmt.Errorf("agent: sending request: %w", err)
	}

	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("agent: reading response: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out != nil && resp.Payload != nil {
		return json.Unmarshal(resp.Payload, out)
	}
	return nil
}

// RequestHTTPDownload asks the agent daemon for a download and returns its header, and for a 2xx response the open body, which the caller must close.
func (c *Client) RequestHTTPDownload(shortID, user, method, path string, body []byte, contentType string, headers []vault.Header) (gatekeeper.DownloadResult, error) {
	timeout := max(c.Timeout, restergate.RequestTimeout)
	conn, err := net.DialTimeout("unix", c.SocketPath, timeout)
	if err != nil {
		return gatekeeper.DownloadResult{}, fmt.Errorf("agent: dialing %s: %w", c.SocketPath, err)
	}
	conn.SetDeadline(time.Now().Add(timeout))

	payload, err := json.Marshal(requestHTTPPayload{ShortID: shortID, User: user, Method: method, Path: path, Body: body, ContentType: contentType, Headers: headers})
	if err == nil {
		err = json.NewEncoder(conn).Encode(request{Op: opRequestHTTPDownload, Payload: payload})
	}
	if err != nil {
		conn.Close()
		return gatekeeper.DownloadResult{}, fmt.Errorf("agent: sending request: %w", err)
	}

	res, err := gatekeeper.ReadDownload(conn, restergate.RequestTimeout)
	if err != nil || res.Body == nil {
		conn.Close()
	}
	var remote gatekeeper.RemoteError
	if err != nil && !errors.As(err, &remote) {
		err = fmt.Errorf("agent: %w", err)
	}
	return res, err
}

func (c *Client) RegisterChannel(realID vault.ChannelID) (string, error) {
	var out shortIDPayload
	err := c.call(opRegisterChannel, registerChannelPayload{RealID: realID}, &out)
	return out.ShortID, err
}

func (c *Client) Query(shortID string, sqlText string) (dbgate.QueryResult, error) {
	var out queryResultPayload
	err := c.call(opQuery, queryPayload{ShortID: shortID, SQL: sqlText}, &out)
	return out.Result, err
}

func (c *Client) Execute(shortID string, sqlText string, commit bool, actor string) (dbgate.WriteResult, error) {
	var out executeResultPayload
	err := c.call(opExecute, executePayload{ShortID: shortID, SQL: sqlText, Commit: commit, Actor: actor}, &out)
	return out.Result, err
}

func (c *Client) RequestHTTP(shortID, user, method, path string, body []byte, contentType string, headers []vault.Header, actor string) (restergate.HTTPResult, error) {
	var out httpResultPayload
	err := c.callWithin(max(c.Timeout, restergate.RequestTimeout), opRequestHTTP, requestHTTPPayload{ShortID: shortID, User: user, Method: method, Path: path, Body: body, ContentType: contentType, Headers: headers, Actor: actor}, &out)
	return out.Result, err
}

func (c *Client) HTTPChannelInfo(shortID string) (restergate.HTTPChannelInfo, error) {
	var out httpChannelInfoPayload
	err := c.call(opHTTPChannelInfo, shortIDPayload{ShortID: shortID}, &out)
	return out.Info, err
}

func (c *Client) HTTPUsers(shortID string) ([]restergate.HTTPUserInfo, error) {
	var out httpUsersPayload
	err := c.call(opHTTPUsers, shortIDPayload{ShortID: shortID}, &out)
	return out.Users, err
}

func (c *Client) FlushHTTPTokens(shortID, user string) (int, error) {
	var out httpFlushedPayload
	err := c.call(opHTTPFlush, httpFlushPayload{ShortID: shortID, User: user}, &out)
	return out.Flushed, err
}

// AppLogin asks the agent to log the channel's app in on the connected phone as user.
func (c *Client) AppLogin(shortID, user, serial string) (restergate.AppLoginResult, error) {
	var out restergate.AppLoginResult
	err := c.callWithin(max(c.Timeout, restergate.AppLoginTimeout), opAppLogin, appLoginPayload{ShortID: shortID, User: user, Serial: serial}, &out)
	return out, err
}

func (c *Client) Forget(shortID string) error {
	return c.call(opForget, shortIDPayload{ShortID: shortID}, nil)
}
