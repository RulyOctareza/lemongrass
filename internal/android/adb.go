// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package android drives an Android device over adb for lgrass device and lgrass flutter.
package android

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const commandTimeout = 30 * time.Second

// Device is one line of adb devices -l.
type Device struct {
	Serial string
	// State is adb's own word: "device" when usable, otherwise e.g. "unauthorized" or "offline".
	State string
	Model string
}

// toolDirs lists where adb is installed besides PATH, since an agent's shell may not have
// platform-tools on its PATH.
func toolDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if dir := os.Getenv(env); dir != "" {
			dirs = append(dirs, filepath.Join(dir, "platform-tools"))
		}
	}
	return append(dirs,
		filepath.Join(home, "Library", "Android", "sdk", "platform-tools"),
		filepath.Join(home, "Android", "Sdk", "platform-tools"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
	)
}

// FindTool returns the path of an executable, from PATH first and then the usual install places.
func FindTool(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, dir := range toolDirs() {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("android: %s is not installed or not on PATH", name)
}

// ParseDevices reads the output of adb devices -l.
func ParseDevices(out string) []Device {
	var devices []Device
	for _, line := range strings.Split(out, "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: fields[0], State: fields[1], Model: fields[0]}
		for _, f := range fields[2:] {
			if model, ok := strings.CutPrefix(f, "model:"); ok {
				d.Model = strings.ReplaceAll(model, "_", "-")
			}
		}
		devices = append(devices, d)
	}
	return devices
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runCommand(ctx, false, name, args...)
}

// runCommand with redact set keeps the arguments and stderr out of the error, for a command
// that carries a typed value such as a password.
func runCommand(ctx context.Context, redact bool, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if redact {
			return nil, fmt.Errorf("android: %s failed: %s", filepath.Base(name), err.Error())
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("android: %s %s: %s", filepath.Base(name), strings.Join(args, " "), detail)
	}
	return stdout.Bytes(), nil
}

// Devices lists what adb sees.
func Devices(ctx context.Context) ([]Device, error) {
	adb, err := FindTool("adb")
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, adb, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return ParseDevices(string(out)), nil
}

// Client runs adb against one device.
type Client struct {
	ADB    string
	Serial string
}

// Open picks the device by serial, or the only usable one when serial is empty.
func Open(ctx context.Context, serial string) (*Client, error) {
	adb, err := FindTool("adb")
	if err != nil {
		return nil, err
	}
	devices, err := Devices(ctx)
	if err != nil {
		return nil, err
	}
	var usable []Device
	for _, d := range devices {
		if serial != "" && d.Serial == serial {
			if d.State != "device" {
				return nil, fmt.Errorf("android: device %s is %s", serial, d.State)
			}
			return &Client{ADB: adb, Serial: serial}, nil
		}
		if d.State == "device" {
			usable = append(usable, d)
		}
	}
	switch {
	case serial != "":
		return nil, fmt.Errorf("android: no device with serial %s is connected", serial)
	case len(usable) == 0:
		return nil, errors.New("android: no Android device is connected. Connect a phone over USB with USB debugging on")
	case len(usable) > 1:
		names := make([]string, len(usable))
		for i, d := range usable {
			names[i] = d.Serial
		}
		return nil, fmt.Errorf("android: several devices are connected (%s), pick one with --device <serial>", strings.Join(names, ", "))
	}
	return &Client{ADB: adb, Serial: usable[0].Serial}, nil
}

// Run runs an adb command against this device.
func (c *Client) Run(ctx context.Context, args ...string) ([]byte, error) {
	return run(ctx, c.ADB, append([]string{"-s", c.Serial}, args...)...)
}

// Shell runs a command in the device shell. Arguments are quoted for the remote shell,
// because adb joins them into one command line there.
func (c *Client) Shell(ctx context.Context, args ...string) ([]byte, error) {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return c.Run(ctx, "shell", strings.Join(quoted, " "))
}

// shellRedacted is Shell for a command line that carries a typed value, whose errors never
// repeat the command.
func (c *Client) shellRedacted(ctx context.Context, args ...string) error {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	_, err := runCommand(ctx, true, c.ADB, "-s", c.Serial, "shell", strings.Join(quoted, " "))
	return err
}

var safeShellWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(s string) string {
	if safeShellWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var sizePattern = regexp.MustCompile(`(\d+)x(\d+)`)

// ScreenSize is the size taps and swipes are measured in, the override size when one is set.
func (c *Client) ScreenSize(ctx context.Context) (int, int, error) {
	out, err := c.Shell(ctx, "wm", "size")
	if err != nil {
		return 0, 0, err
	}
	matches := sizePattern.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return 0, 0, fmt.Errorf("android: unexpected wm size output %q", strings.TrimSpace(string(out)))
	}
	last := matches[len(matches)-1]
	w, _ := strconv.Atoi(last[1])
	h, _ := strconv.Atoi(last[2])
	return w, h, nil
}

// Screenshot writes the current screen as PNG.
func (c *Client) Screenshot(ctx context.Context, path string) error {
	out, err := c.Run(ctx, "exec-out", "screencap", "-p")
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(out, []byte("\x89PNG")) {
		return errors.New("android: screencap did not return a PNG")
	}
	return os.WriteFile(path, out, 0o644)
}

func (c *Client) Tap(ctx context.Context, x, y int) error {
	_, err := c.Shell(ctx, "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

func (c *Client) Swipe(ctx context.Context, x1, y1, x2, y2 int, duration time.Duration) error {
	_, err := c.Shell(ctx, "input", "swipe", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2),
		strconv.Itoa(int(duration.Milliseconds())))
	return err
}

// CheckTypeable reports whether Text can type s. The error gives the position of the first
// character it cannot type, never the character, since s may be a secret.
func CheckTypeable(s string) error {
	for i, r := range []rune(s) {
		if r > 0x7e || (r < 0x20 && r != '\n') {
			return fmt.Errorf("android: character %d is not printable ASCII, and input text types nothing else", i+1)
		}
	}
	return nil
}

// Text types into the focused field. Nothing is typed unless every character can be, and its
// errors never contain the text.
func (c *Client) Text(ctx context.Context, text string) error {
	if err := CheckTypeable(text); err != nil {
		return err
	}
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			if err := c.Key(ctx, "66"); err != nil {
				return err
			}
		}
		for _, chunk := range textChunks(line) {
			if err := c.shellRedacted(ctx, "input", "text", chunk); err != nil {
				return err
			}
		}
	}
	return nil
}

// textChunks splits one line into input text arguments. input text reads every %s as a space
// and has no escape for a literal %s, so spaces are sent as %s and a literal % followed by s
// ends its chunk, which keeps that pair out of any single call. Shell metacharacters need no
// handling here, since Shell quotes each argument.
func textChunks(line string) []string {
	var chunks []string
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			b.WriteString("%s")
			continue
		}
		b.WriteByte(line[i])
		if line[i] == '%' && i+1 < len(line) && line[i+1] == 's' {
			chunks = append(chunks, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}

// KeyNames maps the names lgrass device key accepts to Android KeyEvent codes.
var KeyNames = map[string]string{
	"back":    "4",
	"home":    "3",
	"recents": "187",
	"enter":   "66",
	"del":     "67",
	"tab":     "61",
	"up":      "19",
	"down":    "20",
	"left":    "21",
	"right":   "22",
	"menu":    "82",
	"power":   "26",
}

// Key presses a key by name or by KeyEvent code.
func (c *Client) Key(ctx context.Context, key string) error {
	code, ok := KeyNames[strings.ToLower(key)]
	if !ok {
		if _, err := strconv.Atoi(key); err != nil {
			return fmt.Errorf("android: unknown key %q", key)
		}
		code = key
	}
	_, err := c.Shell(ctx, "input", "keyevent", code)
	return err
}

// ClearField empties the focused single-line field: it moves the cursor to the end and deletes
// chars characters before it, in one input keyevent call. Extra deletes on a shorter or empty
// field do nothing, which keeps this working where input keycombination (Android 13+) is missing.
func (c *Client) ClearField(ctx context.Context, chars int) error {
	args := []string{"input", "keyevent", "123"} // KEYCODE_MOVE_END
	for range chars {
		args = append(args, "67") // KEYCODE_DEL
	}
	_, err := c.Shell(ctx, args...)
	return err
}

// Logcat returns the last lines of the device log, optionally only one tag.
func (c *Client) Logcat(ctx context.Context, lines int, tag string) (string, error) {
	args := []string{"logcat", "-d", "-v", "time", "-t", strconv.Itoa(lines)}
	if tag != "" {
		args = append(args, "-s", tag)
	}
	out, err := c.Run(ctx, args...)
	return string(out), err
}

// Launch starts an app's launcher activity, or brings it to the front when it is running.
func (c *Client) Launch(ctx context.Context, pkg string) error {
	out, err := c.Shell(ctx, "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	if err != nil {
		return err
	}
	// monkey exits 0 on older devices even when nothing launched, and says so on stdout.
	if strings.Contains(string(out), "monkey aborted") {
		return fmt.Errorf("android: %s has no launcher activity on this device, is it installed?", pkg)
	}
	return nil
}

// Dump reads the accessibility tree, which for a Flutter app is its semantics tree.
func (c *Client) Dump(ctx context.Context) ([]Node, error) {
	out, err := c.Run(ctx, "exec-out", "uiautomator", "dump", "/dev/tty")
	if err != nil {
		return nil, err
	}
	return ParseHierarchy(out)
}

// permissionAllowIDs are the allow buttons of Android's runtime permission dialog, newest
// layouts first. Only the system permission controller's own buttons are tapped.
var permissionAllowIDs = []string{
	"com.android.permissioncontroller:id/permission_allow_foreground_only_button",
	"com.android.permissioncontroller:id/permission_allow_button",
	"com.android.packageinstaller:id/permission_allow_button",
}

// AllowPermissionDialog taps Allow when a runtime permission dialog is on screen and reports
// whether it did.
func (c *Client) AllowPermissionDialog(ctx context.Context) (bool, error) {
	nodes, err := c.Dump(ctx)
	if err != nil {
		return false, err
	}
	for _, id := range permissionAllowIDs {
		for _, n := range nodes {
			if n.ID == id {
				x, y := n.Center()
				return true, c.Tap(ctx, x, y)
			}
		}
	}
	return false, nil
}
