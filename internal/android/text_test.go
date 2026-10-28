// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package android

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// androidDecode is what Android's input text does to its argument: every %s becomes a space.
func androidDecode(arg string) string {
	buf := []byte(arg)
	escape := false
	for i := 0; i < len(buf); i++ {
		if escape {
			escape = false
			if buf[i] == 's' {
				buf[i] = ' '
				i--
				buf = append(buf[:i], buf[i+1:]...)
			}
		}
		if buf[i] == '%' {
			escape = true
		}
	}
	return string(buf)
}

var awkward = []string{
	"a&b", "a;b", "$HOME", "$(id)", `say "hi"`, "it's", `back\slash`, "100%", "%s", "%%s",
	"a %s b", "% s", "pass word", " lead", "trail ", `&;$"'\% mixed %s end%`, "-dash", "s%",
}

func TestTextChunksTypeTheLineExactly(t *testing.T) {
	for _, line := range awkward {
		var typed strings.Builder
		for _, chunk := range textChunks(line) {
			if chunk == "" {
				t.Fatalf("textChunks(%q) has an empty chunk", line)
			}
			typed.WriteString(androidDecode(chunk))
		}
		if typed.String() != line {
			t.Errorf("textChunks(%q) types %q", line, typed.String())
		}
	}
}

func TestTextChunksSurviveTheShell(t *testing.T) {
	for _, line := range awkward {
		for _, chunk := range textChunks(line) {
			out, err := exec.Command("sh", "-c", "printf '%s' "+shellQuote(chunk)).Output()
			if err != nil {
				t.Fatalf("sh for %q: %v", chunk, err)
			}
			if string(out) != chunk {
				t.Errorf("shell turned %q into %q", chunk, out)
			}
		}
	}
}

func TestCheckTypeableNamesThePositionNotTheCharacter(t *testing.T) {
	if err := CheckTypeable("line one\nline two ~"); err != nil {
		t.Fatalf("ASCII refused: %v", err)
	}
	for _, s := range []string{"pässword", "tab\there", "emoji😀"} {
		err := CheckTypeable(s)
		if err == nil {
			t.Fatalf("CheckTypeable(%q) accepted", s)
		}
		if strings.ContainsAny(err.Error(), "ä\t😀") {
			t.Errorf("error shows the character: %v", err)
		}
	}
}

func TestTextRefusesBeforeTypingAnything(t *testing.T) {
	// An adb that does not exist fails any command, so a nil-free error proves the check ran first.
	c := &Client{ADB: "/nonexistent/adb", Serial: "x"}
	err := c.Text(context.Background(), "ok then é")
	if err == nil || !strings.Contains(err.Error(), "character 9") {
		t.Fatalf("err = %v", err)
	}
}

func TestRedactedCommandErrorsHideArgsAndStderr(t *testing.T) {
	_, err := runCommand(context.Background(), true, "sh", "-c", "echo hunter2 >&2; exit 3", "hunter2")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err = %v", err)
	}
	_, err = runCommand(context.Background(), false, "sh", "-c", "echo visible >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "visible") {
		t.Fatalf("unredacted err = %v", err)
	}
}
