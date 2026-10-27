// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/faizalv/lemongrass/internal/vault"
)

// HTTPResult is a proxied call's outcome. Body is left as whatever shape the upstream sent --
// parsed JSON when the content type says so, a string otherwise -- since an HTTP body has no
// columns/rows to normalize into the way a db QueryResult does.
type HTTPResult struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    any                 `json:"body"`
	URL     string              `json:"url"`
}

// MaxRequestBodyBytes bounds a proxied request's encoded body. The body crosses two sockets as
// base64 inside one JSON message, so the message is about a third larger.
const MaxRequestBodyBytes = 25 << 20

// CheckContentType reports whether contentType is a single-line type/subtype media type, since
// the value becomes a request header.
func CheckContentType(contentType string) error {
	if strings.ContainsAny(contentType, "\r\n") {
		return errors.New("content type must be a single line")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	if !strings.Contains(mediaType, "/") {
		return errors.New("content type needs a type and a subtype")
	}
	return nil
}

// checkRequestBody rejects a body over MaxRequestBodyBytes and a content type that is not a
// well-formed media type.
func checkRequestBody(body []byte, contentType string) error {
	if len(body) > MaxRequestBodyBytes {
		return fmt.Errorf("restergate: request body is %d bytes, over the %d MiB limit", len(body), MaxRequestBodyBytes>>20)
	}
	if contentType == "" {
		return nil
	}
	if err := CheckContentType(contentType); err != nil {
		return fmt.Errorf("restergate: invalid content type %q: %w", contentType, err)
	}
	return nil
}

// RequestHTTP proxies one call through channel id as user: checks expiry and scope, obtains
// (or reuses) that user's token, and on a non-400 4xx from an already-successfully-used token,
// transparently re-logs in and retries exactly once before giving up. contentType is set only
// for a body read from a file or built from form input, and an empty one means an inline JSON
// body. headers are the caller's own, sent after the rester's.
func (g *Gate) RequestHTTP(id vault.ChannelID, user, method, requestPath string, body []byte, contentType string, headers []vault.Header) (HTTPResult, error) {
	resp, target, err := g.exchange(id, user, method, requestPath, body, contentType, headers, false)
	if err != nil {
		return HTTPResult{}, err
	}
	defer resp.Body.Close()
	return readInline(resp, target.URL)
}

// exchange runs one call through the expiry, scope and token handling that every call shares and returns the open response, whose body the caller must close. A stream call has no total timeout: it waits RequestTimeout for response headers and then RequestTimeout of silence between body reads.
func (g *Gate) exchange(id vault.ChannelID, user, method, requestPath string, body []byte, contentType string, headers []vault.Header, stream bool) (*http.Response, requestTarget, error) {
	c, rester, err := g.openAPIChannel(id)
	if err != nil {
		return nil, requestTarget{}, err
	}

	user, err = resolveUser(rester, user)
	if err != nil {
		return nil, requestTarget{}, err
	}
	target, err := resolveRequestTarget(rester.BaseURL, requestPath)
	if err != nil {
		return nil, requestTarget{}, err
	}
	if !c.Scope.Allow(method, target.ScopePath) {
		return nil, requestTarget{}, fmt.Errorf("restergate: this channel does not grant %s %s", strings.ToUpper(method), target.ScopePath)
	}

	if err := checkRequestBody(body, contentType); err != nil {
		return nil, requestTarget{}, err
	}
	if err := checkCallHeaders(rester, headers); err != nil {
		return nil, requestTarget{}, err
	}

	token, everUsed, err := g.obtainToken(id, rester, user)
	if err != nil {
		return nil, requestTarget{}, err
	}

	resp, err := sendHTTPRequest(rester, token, method, target, body, contentType, headers, stream)
	if err != nil {
		return nil, requestTarget{}, err
	}

	if everUsed && isStaleTokenSignal(resp.StatusCode) {
		resp.Body.Close()
		resp, err = g.retryWithFreshToken(id, rester, user, method, target, body, contentType, headers, stream)
		if err != nil {
			return nil, requestTarget{}, err
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		g.markTokenUsed(id, user)
	}
	return resp, target, nil
}

// retryWithFreshToken evicts the stale cached token, re-logs in (bring-your-own-token users
// have no login to refresh from, so their token is only evicted, not replaced), and retries the
// call exactly once.
func (g *Gate) retryWithFreshToken(id vault.ChannelID, rester vault.ResolvedRester, user, method string, target requestTarget, body []byte, contentType string, headers []vault.Header, stream bool) (*http.Response, error) {
	resterUser, err := findResterUser(rester, user)
	if err != nil {
		return nil, err
	}
	g.evictCachedToken(id, user)
	if resterUser.IsBYOT() {
		return nil, fmt.Errorf("restergate: %s's token appears to have gone stale; ask them to paste a fresh one into the rester and re-save it", vault.UserHandle(user))
	}

	freshToken, _, err := g.refreshToken(id, rester, user)
	if err != nil {
		return nil, fmt.Errorf("restergate: token appeared stale, re-login also failed: %w", err)
	}
	return sendHTTPRequest(rester, freshToken, method, target, body, contentType, headers, stream)
}

// isStaleTokenSignal reports whether status is a non-400 4xx, the signal a used token has gone
// stale server-side (revoked, rotated, or simply expired earlier than its computed TTL implied).
func isStaleTokenSignal(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusBadRequest
}

func sendHTTPRequest(rester vault.ResolvedRester, token, method string, target requestTarget, body []byte, contentType string, headers []vault.Header, stream bool) (*http.Response, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest(strings.ToUpper(method), target.URL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("restergate: building request: %w", err)
	}
	if base, err := url.Parse(rester.BaseURL); err != nil || !strings.EqualFold(req.URL.Host, base.Host) {
		return nil, fmt.Errorf("restergate: refusing to send a request for %s to a host other than %s", target.URL, rester.BaseURL)
	}
	setHeaders(req, rester.Headers)
	setHeaders(req, headers)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	injectToken(req, rester.TokenPlacement, token)

	client := *httpClient
	client.Timeout = RequestTimeout
	client.CheckRedirect = followRedirects(rester)
	if contentType != "" {
		client.CheckRedirect = sameHostRedirectsForFiles
	}

	var idle *idleTimer
	if stream {
		client.Timeout = 0
		idle = newIdleTimer(RequestTimeout)
		req = req.WithContext(idle.ctx)
	}
	resp, err := client.Do(req)
	if err != nil {
		if idle != nil {
			err = idle.explain(err)
			idle.stop()
		}
		return nil, fmt.Errorf("restergate: request failed: %w", err)
	}
	if idle != nil {
		resp.Body = idle.wrap(resp.Body)
	}
	return resp, nil
}

// readInline reads at most maxLoginResponseBytes of resp's body into a result, the shape every non-download call returns.
func readInline(resp *http.Response, requestedURL string) (HTTPResult, error) {
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLoginResponseBytes))
	if err != nil {
		return HTTPResult{}, fmt.Errorf("restergate: reading response: %w", err)
	}
	return HTTPResult{
		Status:  resp.StatusCode,
		Headers: map[string][]string(resp.Header),
		Body:    parseResponseBody(resp.Header.Get("Content-Type"), respBody),
		URL:     requestedURL,
	}, nil
}

func parseResponseBody(contentType string, body []byte) any {
	if strings.Contains(contentType, "json") {
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			return v
		}
	}
	if len(body) == 0 {
		return nil
	}
	return string(body)
}

func injectToken(req *http.Request, placement vault.TokenPlacement, token string) {
	switch placement.Kind {
	case vault.PlacementHeader:
		req.Header.Set(placement.Name, placement.Prefix+token)
	case vault.PlacementCookie:
		req.AddCookie(&http.Cookie{Name: placement.Name, Value: token})
	case vault.PlacementQuery:
		q := req.URL.Query()
		q.Set(placement.Name, token)
		req.URL.RawQuery = q.Encode()
	}
}
