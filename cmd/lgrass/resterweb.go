package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
)

const resterWebUsage = `web channels:
  lgrass rester <short-id> open <path-or-url> [--user <name>]
  lgrass rester <short-id> user <name>
  lgrass rester <short-id> snapshot
  lgrass rester <short-id> click <ref>
  lgrass rester <short-id> type <ref> [--submit] [--] <text>
  lgrass rester <short-id> press <Enter|Tab|Escape>
  lgrass rester <short-id> wait --text <text> | --gone <ref> | --url <pattern>
  lgrass rester <short-id> back
  lgrass rester <short-id> reload
  lgrass rester <short-id> network [--failed] [--since <N>]
  lgrass rester <short-id> console [--errors]
  lgrass rester <short-id> screenshot --out <file-or-directory> [--confirm]

A web channel drives a browser inside the lemongrass app, so these commands only work in a
lemongrass tab. info shows a channel's type: a web channel takes these commands and info and
users, an api channel takes get, post, put, patch, delete, head, options and flush and refuses
these. The human can watch the page in the app's web tab.

open navigates to <path-or-url>, relative to the channel's web origin, logging the user in
first when that user has no session yet. --user names the user as users shows it and may be
left out when the channel has a single web user. user switches the page to another user,
logging in when needed. A snapshot and every action print "as <user>" first, so check who is
acting. When a switch or a login fails no user is active, and snapshot, the actions and open
without --user are refused until user <name> or open <path> --user <name> succeeds, so nothing
runs as the previous user. Passwords and tokens never appear in any output.

snapshot prints the page's URL, title and accessibility tree, each element you can act on
carrying a ref such as e12. open, click, type, press, wait, back and reload are actions: each
prints "action N:", a fresh snapshot, the requests the action caused and console errors.
Actions are numbered from 1 for each user's page. A ref belongs to the snapshot it came from,
so after the page changes take a new snapshot. type fills <ref> with <text>, and --submit
presses Enter after it, and a bare -- before <text> lets it start with dashes. An action waits
for its target to be ready and for the page to settle, and a failure names the condition that
was not met.

wait blocks until the page shows --text, until the element --gone <ref> is gone, or until the
URL matches --url, where * matches any run of characters and the pattern is tried against the
full URL and against the path with its query. Exactly one of them is given.

network lists the requests the page sent with method, URL, status and capped bodies, with auth
headers, cookies and tokens removed. --failed keeps only failed requests and --since <N> only
those from action N onward. console prints console messages and uncaught exceptions, --errors
only the errors.

screenshot saves a PNG of the page to --out and prints {"saved_to": "...", "size": N}. It is
refused while the page shows a password field. A directory gets screenshot.png. A file that
already exists is not replaced unless --confirm is given, and the command stops before taking
the screenshot when it exists.
`

// webCommandSpec says how many positional arguments a web command takes and which flags it accepts. bare flags take no value and are sent as "true".
type webCommandSpec struct {
	args  int
	flags []string
	bare  []string
}

var webCommands = map[string]webCommandSpec{
	"open":       {args: 1, flags: []string{"--user"}},
	"user":       {args: 1},
	"snapshot":   {},
	"click":      {args: 1},
	"type":       {args: 2, bare: []string{"--submit"}},
	"press":      {args: 1},
	"wait":       {flags: []string{"--text", "--gone", "--url"}},
	"back":       {},
	"reload":     {},
	"network":    {flags: []string{"--since"}, bare: []string{"--failed"}},
	"console":    {bare: []string{"--errors"}},
	"screenshot": {flags: []string{"--out"}, bare: []string{"--confirm"}},
}

const screenshotName = "screenshot.png"

var webPressKeys = map[string]string{"enter": "Enter", "tab": "Tab", "escape": "Escape"}

type webCommand struct {
	shortID string
	command string
	args    []string
	flags   map[string]string
}

func isWebCommand(name string) bool {
	_, ok := webCommands[strings.ToLower(name)]
	return ok
}

func parseWebArgs(args []string) (webCommand, error) {
	if len(args) < 2 {
		return webCommand{}, fmt.Errorf("expected a channel id and a command")
	}
	name := strings.ToLower(args[1])
	spec, ok := webCommands[name]
	if !ok {
		return webCommand{}, fmt.Errorf("unknown command %q", args[1])
	}
	cmd := webCommand{shortID: args[0], command: name, args: []string{}, flags: map[string]string{}}

	rest := args[2:]
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		// A bare -- ends the flags, so type can send text that starts with dashes.
		if arg == "--" {
			cmd.args = append(cmd.args, rest[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			cmd.args = append(cmd.args, arg)
			continue
		}
		flag, value, hasValue := strings.Cut(arg, "=")
		key := strings.TrimPrefix(flag, "--")
		switch {
		case contains(spec.bare, flag):
			if hasValue {
				return webCommand{}, fmt.Errorf("%s takes no value", flag)
			}
			cmd.flags[key] = "true"
		case contains(spec.flags, flag):
			if !hasValue {
				i++
				if i >= len(rest) {
					return webCommand{}, fmt.Errorf("%s needs a value", flag)
				}
				value = rest[i]
			}
			if value == "" {
				return webCommand{}, fmt.Errorf("%s needs a value", flag)
			}
			cmd.flags[key] = value
		default:
			return webCommand{}, fmt.Errorf("%s does not take %s", name, flag)
		}
	}

	if len(cmd.args) != spec.args {
		return webCommand{}, fmt.Errorf("%s takes %d argument(s), got %d", name, spec.args, len(cmd.args))
	}
	switch name {
	case "press":
		key, ok := webPressKeys[strings.ToLower(cmd.args[0])]
		if !ok {
			return webCommand{}, fmt.Errorf("press takes Enter, Tab or Escape, not %q", cmd.args[0])
		}
		cmd.args[0] = key
	case "wait":
		if len(cmd.flags) != 1 {
			return webCommand{}, fmt.Errorf("wait needs exactly one of --text, --gone or --url")
		}
	case "screenshot":
		if cmd.flags["out"] == "" {
			return webCommand{}, fmt.Errorf("screenshot needs --out <file-or-directory>")
		}
	}
	return cmd, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// routeResterWeb runs a web command, prints the web usage beside the rester usage, and refuses an api command on a web channel. It returns false when cmdRester should carry on as before.
func routeResterWeb(args []string) bool {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(resterUsage + "\n" + resterWebUsage)
		return true
	}
	if len(args) < 2 {
		return false
	}
	client := &agent.Client{SocketPath: agent.SocketPath()}
	if isWebCommand(args[1]) {
		cmd, err := parseWebArgs(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, resterWebUsage)
			os.Exit(1)
		}
		if err := execWeb(client, cmd, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return true
	}
	if err := refuseAPIOnWeb(client, args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return false
}

// refuseAPIOnWeb reads the channel's type for a request or flush command and refuses it on a web channel. A parse error is left to the ordinary rester path to report. A failed info call is returned, since sending the request anyway would count an unknown id twice in the agent's failure limiter.
func refuseAPIOnWeb(client *agent.Client, args []string) error {
	cmd, err := parseResterArgs(args)
	if err != nil || cmd.info || cmd.users {
		return nil
	}
	info, err := client.HTTPChannelInfo(cmd.shortID)
	if err != nil {
		return err
	}
	if info.Type != "web" {
		return nil
	}
	return fmt.Errorf("this is a web channel, it takes open, user, snapshot, click, type, press, wait, back, reload, network, console and screenshot, not %s. Run lgrass rester --help for their usage", strings.ToLower(args[1]))
}

// execWeb sends cmd through the agent and prints what the model reads. A screenshot is written to --out instead of printed, after the target is checked so an existing file stops the command before the screenshot is taken.
func execWeb(client *agent.Client, cmd webCommand, stdout io.Writer) error {
	var target downloadTarget
	out := cmd.flags["out"]
	confirm := cmd.flags["confirm"] == "true"
	if cmd.command == "screenshot" {
		var err error
		if target, err = checkOutTarget(out, confirm); err != nil {
			return err
		}
		// A screenshot's name is known before the call, so a directory target is checked as the file it will become.
		if target.dir {
			if target, err = checkOutTarget(filepath.Join(target.path, screenshotName), confirm); err != nil {
				return err
			}
		}
	}
	// --out and --confirm are the CLI's own, the engine never writes files.
	flags := map[string]string{}
	for k, v := range cmd.flags {
		if k != "out" && k != "confirm" {
			flags[k] = v
		}
	}
	res, err := client.WebCommand(cmd.shortID, tabID, cmd.command, cmd.args, flags)
	if err != nil {
		return err
	}
	if cmd.command != "screenshot" {
		_, err := io.WriteString(stdout, strings.TrimRight(res.Text, "\n")+"\n")
		return err
	}
	saved, err := saveScreenshot(res.Data, target, confirm)
	if err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{"saved_to": saved.SavedTo, "size": saved.Size})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(b))
	return err
}

// saveScreenshot writes the PNG through saveDownload, so it gets the same existing-file, temporary-file and rename rules as rester's --out.
func saveScreenshot(png []byte, target downloadTarget, confirm bool) (downloadResult, error) {
	if len(png) == 0 {
		return downloadResult{}, fmt.Errorf("the app returned no screenshot, nothing was written")
	}
	res := gatekeeper.DownloadResult{
		Stream: &gatekeeper.StreamMeta{Name: screenshotName, Length: int64(len(png))},
		Body:   io.NopCloser(bytes.NewReader(png)),
	}
	return saveDownload(res, target, confirm)
}
