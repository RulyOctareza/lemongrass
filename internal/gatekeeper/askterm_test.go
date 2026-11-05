// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestTailWriterTerminalOutput(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"colors dropped", "\x1b[1;32mok\x1b[0m\r\n", "ok"},
		{"progress keeps last state", "10%\r50%\r\x1b[K100%\r\ndone\r\n", "100%\ndone"},
		{"crlf split across writes", "a\r", "a"},
		{"osc title dropped", "\x1b]0;title\x07x\x1b]2;t\x1b\\y\r\n", "xy"},
		{"charset pick dropped", "\x1b(Bz\r\n", "z"},
		{"backspace removes a rune", "pé\b\bok\r\n", "ok"},
		{"bell and controls dropped", "a\x07b\tc\r\n", "ab\tc"},
	}
	for _, tc := range cases {
		w := newTailWriter(10, 100)
		w.Write([]byte(tc.in))
		if got := w.String(); got != tc.want {
			t.Errorf("%s: tail = %q, want %q", tc.name, got, tc.want)
		}
	}
	w := newTailWriter(10, 100)
	w.Write([]byte("one\r"))
	w.Write([]byte("\ntwo\r\n"))
	if got := w.String(); got != "one\ntwo" {
		t.Errorf("crlf across writes: tail = %q", got)
	}
}

func startTestTerm(t *testing.T, script string) (*askTerm, *tailWriter, chan struct{}) {
	t.Helper()
	cmd := exec.Command("bash", "-c", script)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: askTermRows, Cols: askTermCols})
	if err != nil {
		t.Fatal(err)
	}
	term := newAskTerm(ptmx)
	tail := newTailWriter(askTailLines, askTailLineBytes)
	pumped := make(chan struct{})
	go func() {
		term.pump(tail)
		close(pumped)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		term.end()
		_ = ptmx.Close()
	})
	return term, tail, pumped
}

func readViewerUntil(t *testing.T, v *askViewer, want string) string {
	t.Helper()
	var seen bytes.Buffer
	deadline := time.After(10 * time.Second)
	for !strings.Contains(seen.String(), want) {
		select {
		case chunk, ok := <-v.out:
			if !ok {
				t.Fatalf("viewer cut off (%s) before %q, saw %q", v.reason, want, seen.String())
			}
			seen.Write(chunk)
		case <-deadline:
			t.Fatalf("no %q in %q", want, seen.String())
		}
	}
	return seen.String()
}

func TestAskTermInputReachesCommand(t *testing.T) {
	term, tail, pumped := startTestTerm(t, `printf 'Proceed? '; read x; echo "got:$x"`)
	replay, v, err := term.attach()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replay), "Proceed?") {
		readViewerUntil(t, v, "Proceed?")
	}
	if err := term.input(v, []byte("yes\r")); err != nil {
		t.Fatal(err)
	}
	readViewerUntil(t, v, "got:yes")
	select {
	case <-pumped:
	case <-time.After(10 * time.Second):
		t.Fatal("output did not end after the command exited")
	}
	if got := tail.String(); got != "Proceed? yes\ngot:yes" {
		t.Fatalf("tail = %q", got)
	}
}

func TestAskTermAttachReplaysAndTakesOver(t *testing.T) {
	term, _, _ := startTestTerm(t, `echo first; read x; echo "second:$x"; sleep 30`)
	_, first, err := term.attach()
	if err != nil {
		t.Fatal(err)
	}
	readViewerUntil(t, first, "first")

	replay, second, err := term.attach()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replay), "first") {
		t.Fatalf("replay = %q, want the earlier output", replay)
	}
	for range first.out {
	}
	if first.reason != askViewReplaced {
		t.Fatalf("first viewer reason = %q", first.reason)
	}
	if err := term.input(first, []byte("x\r")); err == nil {
		t.Fatal("a replaced viewer still wrote input")
	}
	if err := term.resize(second, 40, 120); err != nil {
		t.Fatal(err)
	}
	if err := term.input(second, []byte("ok\r")); err != nil {
		t.Fatal(err)
	}
	readViewerUntil(t, second, "second:ok")

	term.end()
	for range second.out {
	}
	if second.reason != askViewEnded {
		t.Fatalf("second viewer reason = %q", second.reason)
	}
	if _, _, err := term.attach(); err == nil {
		t.Fatal("attached after the end")
	}
}

func TestAskTermReplayIsCapped(t *testing.T) {
	term := &askTerm{}
	line := []byte(strings.Repeat("x", 99) + "\n")
	for range (askReplayBytes / len(line)) + 50 {
		term.record(line)
	}
	if len(term.replay) > askReplayBytes {
		t.Fatalf("replay is %d bytes, cap %d", len(term.replay), askReplayBytes)
	}
	if term.replay[0] != 'x' || term.replay[len(term.replay)-1] != '\n' {
		t.Fatal("replay does not start at a line")
	}
}
