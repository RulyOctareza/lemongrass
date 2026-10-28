// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package flutter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	redacted     = "[redacted]"
	maxBodyBytes = 4096
)

// sensitiveHeaders never reach the caller, since an agent reading the profile must not see a
// credential the app sent or received.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
}

var sensitiveField = regexp.MustCompile(`(?i)password|passwd|token|secret|otp|pin$`)

type profileHalf struct {
	Headers    map[string][]string `json:"headers"`
	StatusCode int                 `json:"statusCode"`
	Error      string              `json:"error"`
	EndTime    int64               `json:"endTime"`
}

// HTTPCall is one request of the app's dart:io HTTP profile.
type HTTPCall struct {
	ID        string       `json:"-"`
	RawID     any          `json:"id"`
	Method    string       `json:"method"`
	URI       string       `json:"uri"`
	StartTime int64        `json:"startTime"`
	EndTime   int64        `json:"endTime"`
	Request   *profileHalf `json:"request"`
	Response  *profileHalf `json:"response"`
	ReqBody   []byte       `json:"-"`
	RespBody  []byte       `json:"-"`
	isolate   string
}

func (c *HTTPCall) normalize(isolate string) {
	c.isolate = isolate
	switch id := c.RawID.(type) {
	case string:
		c.ID = id
	case float64:
		c.ID = fmt.Sprintf("%.0f", id)
	default:
		c.ID = fmt.Sprint(id)
	}
}

// Status is the HTTP status, or what went wrong before one arrived.
func (c HTTPCall) Status() string {
	switch {
	case c.Response != nil && c.Response.Error != "":
		return "ERROR " + c.Response.Error
	case c.Request != nil && c.Request.Error != "":
		return "ERROR " + c.Request.Error
	case c.Response != nil && c.Response.StatusCode > 0:
		return fmt.Sprint(c.Response.StatusCode)
	case c.EndTime == 0:
		return "pending"
	default:
		return "no response"
	}
}

// Failed is true for a transport error or a status of 400 and above.
func (c HTTPCall) Failed() bool {
	s := c.Status()
	return strings.HasPrefix(s, "ERROR") || (c.Response != nil && c.Response.StatusCode >= 400)
}

// end is when the response finished; the call's own endTime marks only the request being sent.
func (c HTTPCall) end() int64 {
	if c.Response != nil && c.Response.EndTime > c.EndTime {
		return c.Response.EndTime
	}
	return c.EndTime
}

// Duration is how long the call took, zero while it is pending.
func (c HTTPCall) Duration() time.Duration {
	if c.end() == 0 {
		return 0
	}
	return time.Duration(c.end()-c.StartTime) * time.Microsecond
}

// HTTPProfile lists the calls of every isolate, newest first.
func (v *VM) HTTPProfile() ([]HTTPCall, error) {
	isolates, err := v.Isolates()
	if err != nil {
		return nil, err
	}
	var calls []HTTPCall
	for _, iso := range isolates {
		var profile struct {
			Requests []HTTPCall `json:"requests"`
		}
		if err := v.Call("ext.dart.io.getHttpProfile", url.Values{"isolateId": {iso.ID}}, &profile); err != nil {
			continue
		}
		for _, c := range profile.Requests {
			c.normalize(iso.ID)
			calls = append(calls, c)
		}
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].StartTime > calls[j].StartTime })
	return calls, nil
}

// HTTPCallDetail fetches one call with its headers and bodies.
func (v *VM) HTTPCallDetail(id string) (HTTPCall, error) {
	calls, err := v.HTTPProfile()
	if err != nil {
		return HTTPCall{}, err
	}
	for _, c := range calls {
		if c.ID != id {
			continue
		}
		var full struct {
			HTTPCall
			ReqRaw  []int `json:"requestBody"`
			RespRaw []int `json:"responseBody"`
		}
		if err := v.Call("ext.dart.io.getHttpProfileRequest", url.Values{"isolateId": {c.isolate}, "id": {id}}, &full); err != nil {
			return HTTPCall{}, err
		}
		detail := full.HTTPCall
		detail.normalize(c.isolate)
		detail.ReqBody = intsToBytes(full.ReqRaw)
		detail.RespBody = intsToBytes(full.RespRaw)
		return detail, nil
	}
	return HTTPCall{}, fmt.Errorf("flutter: no HTTP call with id %s in the profile", id)
}

func intsToBytes(ints []int) []byte {
	out := make([]byte, len(ints))
	for i, b := range ints {
		out[i] = byte(b)
	}
	return out
}

// FormatHTTPLine prints a call as one line of lgrass flutter http.
func FormatHTTPLine(c HTTPCall) string {
	d := "-"
	if c.end() != 0 {
		d = fmt.Sprintf("%dms", c.Duration().Milliseconds())
	}
	return fmt.Sprintf("%-12s %-6s %-5s %7s  %s", c.ID, c.Method, c.Status(), d, c.URI)
}

// FormatHTTPDetail prints a call with headers and bodies, credentials redacted.
func FormatHTTPDetail(c HTTPCall) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\nstatus: %s\nduration: %dms\n", c.Method, c.URI, c.Status(), c.Duration().Milliseconds())
	if c.Request != nil {
		b.WriteString("\nrequest headers:\n")
		writeHeaders(&b, c.Request.Headers)
	}
	b.WriteString("\nrequest body:\n" + formatBody(c.ReqBody) + "\n")
	if c.Response != nil {
		b.WriteString("\nresponse headers:\n")
		writeHeaders(&b, c.Response.Headers)
	}
	b.WriteString("\nresponse body:\n" + formatBody(c.RespBody) + "\n")
	return b.String()
}

func writeHeaders(b *strings.Builder, headers map[string][]string) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := strings.Join(headers[name], ", ")
		if sensitiveHeaders[strings.ToLower(name)] {
			value = redacted
		}
		fmt.Fprintf(b, "  %s: %s\n", name, value)
	}
}

// formatBody shows JSON with credential-like fields redacted, other text as is, and binary
// as its size only.
func formatBody(body []byte) string {
	if len(body) == 0 {
		return "  (empty)"
	}
	var parsed any
	if json.Unmarshal(body, &parsed) == nil {
		out, _ := json.MarshalIndent(redactJSON(parsed), "  ", "  ")
		return "  " + truncate(string(out))
	}
	if !isText(body) {
		return fmt.Sprintf("  (%d bytes of binary)", len(body))
	}
	return "  " + truncate(string(body))
}

func redactJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			if sensitiveField.MatchString(k) {
				t[k] = redacted
			} else {
				t[k] = redactJSON(inner)
			}
		}
	case []any:
		for i := range t {
			t[i] = redactJSON(t[i])
		}
	}
	return v
}

func isText(body []byte) bool {
	for _, b := range body[:min(len(body), 512)] {
		if b == 0 {
			return false
		}
	}
	return true
}

func truncate(s string) string {
	if len(s) <= maxBodyBytes {
		return s
	}
	return s[:maxBodyBytes] + fmt.Sprintf("\n  ... %d more bytes", len(s)-maxBodyBytes)
}
