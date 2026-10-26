// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/android"
)

const deviceUsage = `usage: lgrass device [--device <serial>] <command>

  list                               Connected Android devices
  screenshot [--out <path>]          Saves the screen as PNG and prints its path and size
  tree [--all]                       The on-screen elements with their tap point, label and id;
                                     a Flutter app shows its semantics labels here
  tap <x> <y>                        Taps a point in screen pixels
  tap --text "<label>" [--wait 5s]   Taps the element with this label, waiting for it to appear
  tap --id <id> [--wait 5s]          Taps the element with this resource id (a Flutter Semantics identifier)
  swipe <x1> <y1> <x2> <y2> [--ms 300]
  type "<text>"                      Types into the focused field, printable ASCII only
  key <back|home|recents|enter|del|tab|up|down|left|right|menu|power|<keycode>>
  logs [--lines 200] [--tag <tag>]   The end of the device log; --tag flutter shows the app's prints

--device is needed only when more than one device is connected.`

// cliFlags splits arguments into positionals and --flag values. Flags in valued take the next
// argument; any other flag is a switch.
type cliFlags struct {
	positional []string
	values     map[string]string
	switches   map[string]bool
}

func parseCLIFlags(args []string, valued ...string) (cliFlags, error) {
	f := cliFlags{values: map[string]string{}, switches: map[string]bool{}}
	takesValue := map[string]bool{}
	for _, name := range valued {
		takesValue[name] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			f.positional = append(f.positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if !takesValue[name] {
			f.switches[name] = true
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return f, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		f.values[name] = value
	}
	return f, nil
}

func (f cliFlags) duration(name string, fallback time.Duration) (time.Duration, error) {
	raw, ok := f.values[name]
	if !ok {
		return fallback, nil
	}
	if ms, err := strconv.Atoi(raw); err == nil {
		return time.Duration(ms) * time.Millisecond, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("--%s %q is not a duration such as 5s or 300ms", name, raw)
	}
	return d, nil
}

func ints(args []string, n int) ([]int, error) {
	if len(args) != n {
		return nil, fmt.Errorf("expected %d numbers, got %d", n, len(args))
	}
	out := make([]int, n)
	for i, a := range args {
		v, err := strconv.Atoi(a)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", a)
		}
		out[i] = v
	}
	return out, nil
}

func cmdDevice(args []string) {
	if err := runDevice(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if strings.HasPrefix(err.Error(), "usage") {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func runDevice(ctx context.Context, args []string) error {
	f, err := parseCLIFlags(args, "device", "out", "text", "id", "wait", "ms", "lines", "tag")
	if err != nil {
		return err
	}
	if len(f.positional) == 0 {
		return errors.New(deviceUsage)
	}
	command, rest := f.positional[0], f.positional[1:]
	if command == "list" {
		return deviceList(ctx)
	}
	client, err := android.Open(ctx, f.values["device"])
	if err != nil {
		return err
	}
	switch command {
	case "screenshot":
		return deviceScreenshot(ctx, client, f.values["out"])
	case "tree":
		return deviceTree(ctx, client, f.switches["all"])
	case "tap":
		return deviceTap(ctx, client, f, rest)
	case "swipe":
		p, err := ints(rest, 4)
		if err != nil {
			return fmt.Errorf("swipe: %w", err)
		}
		d, err := f.duration("ms", 300*time.Millisecond)
		if err != nil {
			return err
		}
		if err := client.Swipe(ctx, p[0], p[1], p[2], p[3], d); err != nil {
			return err
		}
		fmt.Printf("swiped %d,%d to %d,%d\n", p[0], p[1], p[2], p[3])
		return nil
	case "type":
		if len(rest) != 1 {
			return errors.New(`usage: lgrass device type "<text>"`)
		}
		if err := client.Text(ctx, rest[0]); err != nil {
			return err
		}
		fmt.Printf("typed %d characters\n", len([]rune(rest[0])))
		return nil
	case "key":
		if len(rest) != 1 {
			return errors.New("usage: lgrass device key <name|keycode>")
		}
		if err := client.Key(ctx, rest[0]); err != nil {
			return err
		}
		fmt.Printf("pressed %s\n", rest[0])
		return nil
	case "logs":
		lines := 200
		if raw, ok := f.values["lines"]; ok {
			if lines, err = strconv.Atoi(raw); err != nil || lines <= 0 {
				return fmt.Errorf("--lines %q is not a positive number", raw)
			}
		}
		out, err := client.Logcat(ctx, lines, f.values["tag"])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	default:
		return fmt.Errorf("unknown device command %q\n\n%s", command, deviceUsage)
	}
}

func deviceList(ctx context.Context) error {
	devices, err := android.Devices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("lgrass: no Android device is connected.")
		return nil
	}
	for _, d := range devices {
		fmt.Printf("%s  %s  %s\n", d.Serial, d.State, d.Model)
	}
	return nil
}

func deviceScreenshot(ctx context.Context, client *android.Client, out string) error {
	if out == "" {
		// The temp dir fits: a screenshot is cheap to take again.
		dir := filepath.Join(os.TempDir(), "lgrass-device")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		out = filepath.Join(dir, fmt.Sprintf("%s-%d.png", client.Serial, time.Now().UnixMilli()))
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := client.Screenshot(ctx, abs); err != nil {
		return err
	}
	w, h, err := client.ScreenSize(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s\nscreen %dx%d, tap coordinates are in these pixels\n", abs, w, h)
	return nil
}

func deviceTree(ctx context.Context, client *android.Client, all bool) error {
	nodes, err := client.Dump(ctx)
	if err != nil {
		return err
	}
	if !all {
		nodes = android.Visible(nodes)
	}
	w, h, err := client.ScreenSize(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("screen %dx%d. Each line: (tap x,y) \"label\" id=<id> tap=clickable [bounds]\n", w, h)
	for _, n := range nodes {
		fmt.Println(android.Format(n))
	}
	return nil
}

func deviceTap(ctx context.Context, client *android.Client, f cliFlags, rest []string) error {
	text, byText := f.values["text"]
	id, byID := f.values["id"]
	if !byText && !byID {
		p, err := ints(rest, 2)
		if err != nil {
			return fmt.Errorf("tap: %w. Use tap <x> <y>, tap --text \"<label>\" or tap --id <id>", err)
		}
		if err := client.Tap(ctx, p[0], p[1]); err != nil {
			return err
		}
		fmt.Printf("tapped %d,%d\n", p[0], p[1])
		return nil
	}
	wait, err := f.duration("wait", 5*time.Second)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(wait)
	for {
		nodes, err := client.Dump(ctx)
		if err != nil {
			return err
		}
		var node android.Node
		if byID {
			node, err = android.FindByID(nodes, id)
		} else {
			node, err = android.FindByText(nodes, text)
		}
		if err == nil {
			x, y := node.Center()
			if err := client.Tap(ctx, x, y); err != nil {
				return err
			}
			fmt.Printf("tapped %q at %d,%d\n", strings.ReplaceAll(node.Label(), "\n", " / "), x, y)
			return nil
		}
		if !errors.Is(err, android.ErrNotFound) || time.Now().After(deadline) {
			return withLabels(err, nodes)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// withLabels lists what is on screen, so a failed tap tells the caller what it could tap instead.
func withLabels(err error, nodes []android.Node) error {
	var labels []string
	for _, n := range android.Visible(nodes) {
		if label := n.Label(); label != "" {
			labels = append(labels, strconv.Quote(strings.ReplaceAll(label, "\n", " / ")))
		}
		if len(labels) == 30 {
			break
		}
	}
	if len(labels) == 0 {
		return err
	}
	return fmt.Errorf("%w\non screen: %s", err, strings.Join(labels, ", "))
}
