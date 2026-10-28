// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

const resterUsage = `usage:
  lgrass rester <short-id> info
  lgrass rester <short-id> users
  lgrass rester <short-id> flush [--user <name>]
  lgrass rester <short-id> <get|post|put|patch|delete|head|options> <path> [--user <name>] [--header '<name>: <value>']... [body flags]
  lgrass rester <short-id> <get|post|put|patch|delete> <path> --out <file-or-directory> [--confirm]

body flags, at most one kind per request:
  --body '<json>'                       an inline JSON body
  --body-file <path> [--content-type <mime>]
                                        the file's bytes as the body, JSON unless --content-type says otherwise
  --file <field>=<path>[;type=<mime>]   a file as one part of a multipart/form-data body, repeatable
  --form <key>=<value>                  a form field, repeatable; with --file it joins the multipart body,
                                        alone it makes an application/x-www-form-urlencoded body

info prints the channel's rester, environment and type, its base URL, how long it stays valid,
the methods it allows and its users. users prints just the users with their tags. The request
form proxies one HTTP call through the channel's rester; the vault handles login and token injection. <path> is relative
to the environment's base URL, a full URL under it also works. --user may be left out when the
channel has a single user. A user is named as users shows it: lowercase, with underscores for
spaces, and a name typed in another case or with spaces reaches the same user. Response is printed as JSON:
{"status": N, "headers": {...}, "body": ..., "url": "..."}.

--header adds one request header to this call and may be repeated, with or without --out. The
rester may already send headers of its own, which are not shown and cannot be replaced. A header
that carries the token or a credential (Authorization, Cookie), Content-Type (use --content-type
with --body-file), or one the rester sets is refused with the header named.

--out saves the response body of a successful call to a file instead of printing it, byte for
byte and with no size limit, and prints {"status": N, "headers": {...}, "size": N, "url": "...",
"saved_to": "..."} without the file's bytes. <file-or-directory> may be any place this user can
write. A directory gets the file name the server suggests. A target that already exists is not
replaced unless --confirm is given, and the command stops before making any request when the
target is a file that exists. A response that is not 2xx is printed as usual and nothing is
written. The file is written to a temporary name beside the target and renamed into place only
when the whole body arrived, so a failed download leaves an existing file untouched.

Files are read from this machine as the current user and sent as they are, so a spreadsheet
arrives byte for byte. A part's type comes from the file extension unless ;type= is given. A
request body is limited to 25 MiB. A redirect to another host is not followed for a request
that sends a file, so the file is never resent elsewhere.

The vault keeps each user's login token until it expires and reuses it across requests. flush
drops the cached token of --user, or of every user when --user is left out, so the next request
logs in again and gets a new token. It prints {"flushed": N}, the number of tokens dropped. A
user whose token was pasted in by hand has no login, so it gets the same token back.
`

var resterVerbs = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true,
}

type resterCommand struct {
	shortID string
	info    bool
	users   bool
	flush   bool
	method  string
	path    string
	user    string
	body    string
	out     string
	confirm bool

	bodyFile    string
	contentType string
	forms       []formField
	files       []filePart
	headers     []vault.Header
}

func parseResterArgs(args []string) (resterCommand, error) {
	if len(args) < 2 {
		return resterCommand{}, fmt.Errorf("expected a channel id and a command")
	}
	cmd := resterCommand{shortID: args[0]}

	action := strings.ToLower(args[1])
	switch {
	case action == "info":
		cmd.info = true
	case action == "users":
		cmd.users = true
	case action == "flush":
		cmd.flush = true
	case resterVerbs[action]:
		cmd.method = strings.ToUpper(action)
	default:
		return resterCommand{}, fmt.Errorf("unknown command %q", args[1])
	}

	var positional []string
	rest := args[2:]
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if !strings.HasPrefix(arg, "--") {
			positional = append(positional, arg)
			continue
		}
		if arg == "--confirm" {
			cmd.confirm = true
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if !hasValue {
			i++
			if i >= len(rest) {
				return resterCommand{}, fmt.Errorf("%s needs a value", name)
			}
			value = rest[i]
		}
		switch name {
		case "--user":
			cmd.user = value
		case "--out":
			cmd.out = value
		case "--body":
			cmd.body = value
		case "--body-file":
			cmd.bodyFile = value
		case "--content-type":
			cmd.contentType = value
		case "--form":
			field, err := parseFormField(value)
			if err != nil {
				return resterCommand{}, err
			}
			cmd.forms = append(cmd.forms, field)
		case "--file":
			part, err := parseFilePart(value)
			if err != nil {
				return resterCommand{}, err
			}
			cmd.files = append(cmd.files, part)
		case "--header":
			h, err := parseHeader(value)
			if err != nil {
				return resterCommand{}, err
			}
			cmd.headers = append(cmd.headers, h)
		default:
			return resterCommand{}, fmt.Errorf("unknown flag %s", name)
		}
	}

	if cmd.info || cmd.users {
		if len(positional) > 0 || cmd.user != "" || cmd.hasBodyFlags() || cmd.hasOutFlags() || len(cmd.headers) > 0 {
			return resterCommand{}, fmt.Errorf("%s takes no other arguments", strings.ToLower(args[1]))
		}
		return cmd, nil
	}
	if cmd.flush {
		if len(positional) > 0 || cmd.hasBodyFlags() || cmd.hasOutFlags() || len(cmd.headers) > 0 {
			return resterCommand{}, fmt.Errorf("flush takes only --user")
		}
		return cmd, nil
	}
	if len(positional) != 1 {
		return resterCommand{}, fmt.Errorf("%s needs exactly one path", strings.ToLower(cmd.method))
	}
	cmd.path = positional[0]
	if err := cmd.checkBodyFlags(); err != nil {
		return resterCommand{}, err
	}
	if err := cmd.checkOutFlags(); err != nil {
		return resterCommand{}, err
	}
	return cmd, nil
}

// parseHeader reads a --header value written as name: value.
func parseHeader(value string) (vault.Header, error) {
	name, v, ok := strings.Cut(value, ":")
	if !ok {
		return vault.Header{}, fmt.Errorf("--header needs '<name>: <value>', got %q", value)
	}
	h := vault.Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(v)}
	if err := vault.CheckHeader(h); err != nil {
		return vault.Header{}, fmt.Errorf("--header: %w", err)
	}
	return h, nil
}

func (c resterCommand) hasOutFlags() bool {
	return c.out != "" || c.confirm
}

// checkOutFlags rejects --confirm without --out and --out on a verb whose response has no body to save.
func (c resterCommand) checkOutFlags() error {
	if c.confirm && c.out == "" {
		return fmt.Errorf("--confirm only goes with --out")
	}
	if c.out != "" && (c.method == "HEAD" || c.method == "OPTIONS") {
		return fmt.Errorf("--out needs a call that returns a body, not %s", strings.ToLower(c.method))
	}
	return nil
}

func (c resterCommand) hasBodyFlags() bool {
	return c.body != "" || c.bodyFile != "" || c.contentType != "" || len(c.forms) > 0 || len(c.files) > 0
}

// checkBodyFlags rejects a request that mixes body kinds or gives --content-type without a
// file to describe.
func (c resterCommand) checkBodyFlags() error {
	kinds := 0
	for _, used := range []bool{c.body != "", c.bodyFile != "", len(c.forms) > 0 || len(c.files) > 0} {
		if used {
			kinds++
		}
	}
	if kinds > 1 {
		return fmt.Errorf("use only one kind of body: --body, --body-file, or --form and --file")
	}
	if c.contentType != "" && c.bodyFile == "" {
		return fmt.Errorf("--content-type only goes with --body-file")
	}
	if c.contentType != "" {
		if err := restergate.CheckContentType(c.contentType); err != nil {
			return fmt.Errorf("--content-type %q is not a valid media type", c.contentType)
		}
	}
	return nil
}

func cmdRester(args []string) {
	if routeResterApp(args) || routeResterWeb(args) {
		return
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(resterUsage)
		return
	}

	cmd, err := parseResterArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, resterUsage)
		os.Exit(1)
	}

	result, err := execRester(&agent.Client{SocketPath: agent.SocketPath()}, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}

func execRester(client *agent.Client, cmd resterCommand) (any, error) {
	switch {
	case cmd.info:
		return client.HTTPChannelInfo(cmd.shortID)
	case cmd.users:
		return client.HTTPUsers(cmd.shortID)
	case cmd.flush:
		flushed, err := client.FlushHTTPTokens(cmd.shortID, cmd.user)
		return map[string]int{"flushed": flushed}, err
	}

	var target downloadTarget
	if cmd.out != "" {
		var err error
		if target, err = checkOutTarget(cmd.out, cmd.confirm); err != nil {
			return nil, err
		}
	}
	body, contentType, err := buildResterBody(cmd)
	if err != nil {
		return nil, err
	}
	if cmd.out != "" {
		return downloadThroughAgent(client, cmd, target, body, contentType)
	}
	return client.RequestHTTP(cmd.shortID, cmd.user, cmd.method, cmd.path, body, contentType, cmd.headers, actorFor(cmd.shortID))
}
