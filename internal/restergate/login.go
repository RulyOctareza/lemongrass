package restergate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// TestResterLogin attempts a real login for user against the api side's LoginEndpoint at the
// environment's base URL, or validates a bring-your-own-token user's pasted token, with no root
// secret and no stored state involved -- the same fresh-value check TestConnection does for a db
// connection string. user is the effective user the form holds.
func (g *Gate) TestResterLogin(api vault.ResterAPI, env vault.Environment, user vault.ResterUser) error {
	rester := vault.ResolvedRester{
		Environment:     env.Name,
		BaseURL:         env.BaseURL,
		LoginEndpoint:   api.LoginEndpoint,
		TokenPath:       api.TokenPath,
		TTLOrigin:       api.TTLOrigin,
		FixedTTLSeconds: api.FixedTTLSeconds,
		TokenPlacement:  api.TokenPlacement,
		Headers:         vault.MergeHeaders(api.Headers, env.Headers),
	}
	if user.IsBYOT() {
		_, err := byotExpiry(rester, user)
		return err
	}
	_, _, err := loginHTTP(rester, user)
	return err
}

// LoginStatusError is a non-2xx login response. Its message carries only the status, since the
// body can echo the submitted fields and reaches the model through every caller but the Vault UI.
type LoginStatusError struct {
	Status int
	Body   []byte
}

func (e *LoginStatusError) Error() string {
	return fmt.Sprintf("restergate: login returned status %d", e.Status)
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// loginHTTP POSTs user.LoginBody() to rester.LoginEndpoint and extracts the token and
// its computed expiry from the response.
func loginHTTP(rester vault.ResolvedRester, user vault.ResterUser) (token string, expiresAt time.Time, err error) {
	if rester.LoginEndpoint == "" {
		return "", time.Time{}, errors.New("restergate: rester has no login endpoint, only bring-your-own-token users are supported")
	}
	body, err := user.LoginBody()
	if err != nil {
		return "", time.Time{}, err
	}
	req, err := http.NewRequest(http.MethodPost, rester.BaseURL+rester.LoginEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("restergate: building login request: %w", err)
	}
	setHeaders(req, rester.Headers)
	req.Header.Set("Content-Type", "application/json")

	client := *httpClient
	client.CheckRedirect = sameHostRedirectsOnly
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("restergate: logging in: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLoginResponseBytes))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("restergate: reading login response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, &LoginStatusError{Status: resp.StatusCode, Body: respBody}
	}

	token, err = jsonPathString(respBody, rester.TokenPath)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("restergate: extracting token: %w", err)
	}

	expiresAt, err = ttlFromLoginResponse(rester, respBody, token)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// byotExpiry computes a bring-your-own-token user's expiry: from the token itself when
// TTLOrigin is "jwt-exp", otherwise from FixedTTLSeconds, since there's no login response to
// pull a JSON-path TTL from.
func byotExpiry(rester vault.ResolvedRester, user vault.ResterUser) (time.Time, error) {
	if user.Token == "" {
		return time.Time{}, errors.New("restergate: bring-your-own-token user has no token")
	}
	switch rester.TTLOrigin {
	case "jwt-exp":
		return jwtExpiry(user.Token)
	case "":
		return time.Now().Add(time.Duration(rester.FixedTTLSeconds) * time.Second), nil
	default:
		return time.Time{}, fmt.Errorf("restergate: TTLOrigin %q needs a login response, which a bring-your-own-token user never has -- use \"jwt-exp\" or a fixed TTL", rester.TTLOrigin)
	}
}

// ttlFromLoginResponse computes a login user's expiry from TTLOrigin: a JSON path into
// respBody, the literal "jwt-exp" to decode token itself, or FixedTTLSeconds when TTLOrigin is empty.
func ttlFromLoginResponse(rester vault.ResolvedRester, respBody []byte, token string) (time.Time, error) {
	switch rester.TTLOrigin {
	case "":
		return time.Now().Add(time.Duration(rester.FixedTTLSeconds) * time.Second), nil
	case "jwt-exp":
		return jwtExpiry(token)
	default:
		raw, err := jsonPathString(respBody, rester.TTLOrigin)
		if err != nil {
			return time.Time{}, fmt.Errorf("restergate: extracting TTL: %w", err)
		}
		seconds, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("restergate: TTLOrigin %q did not yield a number: %w", rester.TTLOrigin, err)
		}
		return time.Now().Add(time.Duration(seconds * float64(time.Second))), nil
	}
}

// jwtExpiry decodes a JWT's payload segment (no signature verification -- the vault trusts the
// issuing rester it just logged into, not the token as an untrusted credential) and reads its
// numeric exp claim.
func jwtExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("restergate: token is not a JWT (expected 3 dot-separated parts, got %d)", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("restergate: decoding JWT payload: %w", err)
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("restergate: decoding JWT claims: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, errors.New("restergate: JWT has no exp claim")
	}
	return time.Unix(int64(claims.Exp), 0), nil
}

// jsonPathString reads a dot-separated path (e.g. "data.token") out of a JSON object and
// renders the leaf as a string, whatever its JSON type.
func jsonPathString(data []byte, path string) (string, error) {
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return "", fmt.Errorf("restergate: decoding JSON: %w", err)
	}
	node := tree
	for _, key := range strings.Split(path, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return "", fmt.Errorf("restergate: path %q: %q is not an object", path, key)
		}
		val, ok := m[key]
		if !ok {
			return "", fmt.Errorf("restergate: path %q: %q not found", path, key)
		}
		node = val
	}
	switch v := node.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("restergate: path %q resolved to an unsupported type %T", path, node)
	}
}

const maxLoginResponseBytes = 1 << 20 // 1MiB, generous for a login response
