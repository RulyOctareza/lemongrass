// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// requestTarget is a caller-supplied path resolved onto a rester's base URL. ScopePath is the
// cleaned, decoded path relative to the base, the form scope is checked against and the form
// the server sees; URL is the full address the request is sent to.
type requestTarget struct {
	ScopePath string
	URL       string
}

// resolveRequestTarget turns whatever a caller passed as a path into a request target that can
// only address baseURL's own host. It accepts a plain path, a path missing its leading slash,
// a path repeating the base URL's path prefix, and a full URL under the base. Anything that
// would leave the base host, or that is ambiguous about where it points, is an error.
func resolveRequestTarget(baseURL, input string) (requestTarget, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" {
		return requestTarget{}, fmt.Errorf("restergate: rester base URL %q is not a valid URL", baseURL)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return requestTarget{}, errors.New("restergate: request path is empty")
	}
	if strings.HasPrefix(input, "//") {
		return requestTarget{}, fmt.Errorf("restergate: path %q looks like a scheme-relative URL; pass a path such as /orders or a full URL under %s", input, baseURL)
	}

	absolute := strings.Contains(input, "://")
	if !absolute && !strings.HasPrefix(input, "/") {
		input = "/" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		return requestTarget{}, fmt.Errorf("restergate: path %q is not valid: %w", input, err)
	}

	basePath := strings.TrimRight(base.Path, "/")
	if absolute {
		if !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) || u.User != nil {
			return requestTarget{}, fmt.Errorf("restergate: %q is not under this rester's base URL %s; pass a path or a URL on that host", input, baseURL)
		}
		if basePath != "" && u.Path != basePath && !strings.HasPrefix(u.Path, basePath+"/") {
			return requestTarget{}, fmt.Errorf("restergate: %q is outside this rester's base path %s", input, basePath)
		}
	}

	lowerEscaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(lowerEscaped, "%2f") || strings.Contains(lowerEscaped, "%5c") || strings.ContainsAny(u.Path, "\\\x00") {
		return requestTarget{}, errors.New("restergate: path contains an encoded slash, a backslash or a NUL byte, which are not accepted")
	}

	scopePath := path.Clean(u.Path)
	if strings.HasSuffix(u.Path, "/") && scopePath != "/" {
		scopePath += "/"
	}
	if basePath != "" && (scopePath == basePath || strings.HasPrefix(scopePath, basePath+"/")) {
		scopePath = strings.TrimPrefix(scopePath, basePath)
		if scopePath == "" {
			scopePath = "/"
		}
	}

	target := *base
	target.Path = basePath + scopePath
	target.RawPath = ""
	target.RawQuery = u.RawQuery
	target.Fragment = ""
	return requestTarget{ScopePath: scopePath, URL: target.String()}, nil
}
