// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import (
	"errors"
	"net/http"
	"strings"

	"github.com/faizalv/lemongrass/internal/vault"
)

const maxRedirects = 10

// followRedirects returns a redirect policy for a proxied call: a redirect to another host is
// followed, but with the token stripped from wherever the rester's placement put it, the rester's
// own headers, cookies and the referrer (which carries a query-placed token) dropped, so a
// presigned download URL still works and nothing the rester holds reaches a second host.
func followRedirects(rester vault.ResolvedRester) func(req *http.Request, via []*http.Request) error {
	placement := rester.TokenPlacement
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		if strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return nil
		}
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Referer")
		for _, h := range rester.Headers {
			req.Header.Del(h.Name)
		}
		switch placement.Kind {
		case vault.PlacementHeader:
			req.Header.Del(placement.Name)
		case vault.PlacementQuery:
			q := req.URL.Query()
			q.Del(placement.Name)
			req.URL.RawQuery = q.Encode()
		}
		return nil
	}
}

// sameHostRedirectsOnly refuses any redirect to another host. A login call carries the user's
// credentials in its body, which a 307 or 308 would resend to wherever it points.
func sameHostRedirectsOnly(req *http.Request, via []*http.Request) error {
	return refuseCrossHost("login redirected to another host, refusing to resend credentials")(req, via)
}

// sameHostRedirectsForFiles is the redirect policy for a request that sends a file or form body,
// which a 307 or 308 would resend to wherever it points.
func sameHostRedirectsForFiles(req *http.Request, via []*http.Request) error {
	return refuseCrossHost("the request redirected to another host, refusing to resend the file")(req, via)
}

func refuseCrossHost(refusal string) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return errors.New(refusal)
		}
		return nil
	}
}
