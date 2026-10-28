// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/restergate"
)

const resterAppUsage = `app channels:
  lgrass rester <short-id> app-login [--user <name>] [--device <serial>]

An app channel logs a phone app in without showing you a credential. app-login runs the login
steps the human stored for the app on the Android phone connected over USB: it launches the
app, taps, types the user's fields and presses keys. --user names the user as users shows it
and may be left out when the channel has a single user. --device picks a phone by serial when
several are connected. It prints "as <user>" and then ok, or the step that failed, for example
"failed at step 3 (fill): ...", and exits 1 on a failure. Passwords and tokens never appear in
any output. info shows a channel's type and the app's package, an app channel takes app-login,
info and users, and refuses api calls and web commands.
`

type appLoginCommand struct {
	shortID string
	user    string
	device  string
}

func isAppLogin(args []string) bool {
	return len(args) >= 2 && strings.EqualFold(args[1], "app-login")
}

func parseAppLoginArgs(args []string) (appLoginCommand, error) {
	if !isAppLogin(args) {
		return appLoginCommand{}, fmt.Errorf("expected a channel id and app-login")
	}
	cmd := appLoginCommand{shortID: args[0]}
	rest := args[2:]
	for i := 0; i < len(rest); i++ {
		flag, value, hasValue := strings.Cut(rest[i], "=")
		if flag != "--user" && flag != "--device" {
			if strings.HasPrefix(flag, "--") {
				return appLoginCommand{}, fmt.Errorf("app-login does not take %s", flag)
			}
			return appLoginCommand{}, fmt.Errorf("app-login takes only --user and --device, not %q", rest[i])
		}
		if !hasValue {
			i++
			if i >= len(rest) {
				return appLoginCommand{}, fmt.Errorf("%s needs a value", flag)
			}
			value = rest[i]
		}
		if value == "" {
			return appLoginCommand{}, fmt.Errorf("%s needs a value", flag)
		}
		if flag == "--user" {
			cmd.user = value
		} else {
			cmd.device = value
		}
	}
	return cmd, nil
}

// routeResterApp runs app-login and returns true, or returns false when cmdRester should carry on as before.
func routeResterApp(args []string) bool {
	if !isAppLogin(args) {
		return false
	}
	cmd, err := parseAppLoginArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, resterAppUsage)
		os.Exit(1)
	}
	ok, err := execAppLogin(&agent.Client{SocketPath: agent.SocketPath()}, cmd, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !ok {
		os.Exit(1)
	}
	return true
}

// execAppLogin prints what the model reads and reports whether the login succeeded. A failed step is printed to stdout under its "as <user>" line, since the user did act.
func execAppLogin(client *agent.Client, cmd appLoginCommand, stdout io.Writer) (bool, error) {
	res, err := client.AppLogin(cmd.shortID, cmd.user, cmd.device)
	if err != nil {
		return false, err
	}
	_, err = io.WriteString(stdout, formatAppLogin(res))
	return res.OK, err
}

func formatAppLogin(res restergate.AppLoginResult) string {
	if res.OK {
		return fmt.Sprintf("as %s\nok\n", res.User)
	}
	return fmt.Sprintf("as %s\nfailed at step %d (%s): %s\n", res.User, res.FailedStep, res.FailedAction, res.Reason)
}
