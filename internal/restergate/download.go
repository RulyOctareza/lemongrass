// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/faizalv/lemongrass/internal/vault"
)

// Download is the outcome of a download call. A 2xx response has Body open and Inline nil, and the caller must close Body. Any other response is read into Inline with Body nil, the same result RequestHTTP would return.
type Download struct {
	Status  int
	Headers map[string][]string
	URL     string
	// Name is the file name the server suggested or the URL path ends in, reduced to a base name, or empty when neither gives one.
	Name string
	// Length is the declared body size, or -1 when the response did not declare one.
	Length int64
	Body   io.ReadCloser
	Inline *HTTPResult
}

// DownloadHTTP makes the same checked, token-bearing call as RequestHTTP but hands back a 2xx response body unread instead of buffering it, so a body of any size can be streamed to its destination.
func (g *Gate) DownloadHTTP(id vault.ChannelID, user, method, requestPath string, body []byte, contentType string, headers []vault.Header) (Download, error) {
	resp, target, err := g.exchange(id, user, method, requestPath, body, contentType, headers, true)
	if err != nil {
		return Download{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		result, err := readInline(resp, target.URL)
		if err != nil {
			return Download{}, err
		}
		return Download{Status: result.Status, Headers: result.Headers, URL: result.URL, Inline: &result}, nil
	}
	return Download{
		Status:  resp.StatusCode,
		Headers: map[string][]string(resp.Header),
		URL:     target.URL,
		Name:    downloadName(resp.Header, resp.Request.URL.Path),
		Length:  resp.ContentLength,
		Body:    resp.Body,
	}, nil
}

// downloadName picks a file name from Content-Disposition, then the last segment of the final URL path.
func downloadName(header http.Header, finalPath string) string {
	if _, params, err := mime.ParseMediaType(header.Get("Content-Disposition")); err == nil {
		if name := baseFileName(params["filename"]); name != "" {
			return name
		}
	}
	return baseFileName(path.Base(finalPath))
}

// baseFileName reduces a server-supplied name to a plain file name, or "" when nothing safe is left.
func baseFileName(name string) string {
	name = strings.NewReplacer("\\", "/").Replace(name)
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return ""
	}
	return name
}
