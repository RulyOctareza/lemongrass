// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

const (
	// askReplayBytes caps the raw terminal output a running command keeps, so a card that attaches late or again redraws what the human missed.
	askReplayBytes = 256 << 10
	// askReplayResync is how far into the trimmed replay a line start is looked for, so a replay rarely opens in the middle of a line or an escape sequence.
	askReplayResync = 4 << 10
	// askViewerBuffer is how many output chunks a viewer may fall behind before it is cut off; it can attach again and gets the replay.
	askViewerBuffer = 256
	askTermRows     = 24
	askTermCols     = 100
	// askTermMax bounds either side of a terminal size the app sends.
	askTermMax = 1000
	// askInputBytes caps one input write, which comes from a keystroke or a paste.
	askInputBytes = 64 << 10
)

// Reasons a viewer's output ends.
const (
	askViewEnded    = "ended"
	askViewReplaced = "replaced"
	askViewBehind   = "behind"
	askViewDetached = "detached"
)

// askConsole is the terminal a command runs in: a pty on Unix, a ConPTY on Windows.
type askConsole interface {
	io.ReadWriter
	Resize(rows, cols int) error
}

// askTerm is the pseudo-terminal of one running command: everything it prints goes to the tail for the model, to the replay and to the one attached viewer, and the viewer's keystrokes go back in.
type askTerm struct {
	mu     sync.Mutex
	pty    askConsole
	replay []byte
	viewer *askViewer
	ended  bool
}

// askViewer receives a running command's output until out is closed; reason says why, and is set before the close.
type askViewer struct {
	out    chan []byte
	reason string
}

func newAskTerm(console askConsole) *askTerm {
	return &askTerm{pty: console}
}

// pump copies the terminal's output until it closes, which on Linux is an error once every holder of the terminal has exited.
func (t *askTerm) pump(tail io.Writer) {
	buf := make([]byte, 32<<10)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			_, _ = tail.Write(chunk)
			t.record(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (t *askTerm) record(chunk []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.replay = append(t.replay, chunk...)
	if len(t.replay) > askReplayBytes {
		trimmed := t.replay[len(t.replay)-askReplayBytes:]
		if i := bytes.IndexByte(trimmed[:min(len(trimmed), askReplayResync)], '\n'); i >= 0 {
			trimmed = trimmed[i+1:]
		}
		t.replay = append([]byte(nil), trimmed...)
	}
	if t.viewer == nil {
		return
	}
	select {
	case t.viewer.out <- chunk:
	default:
		t.cutLocked(askViewBehind)
	}
}

// attach makes v the one viewer, cutting off any earlier one, and returns the output so far. It fails once the command has ended.
func (t *askTerm) attach() ([]byte, *askViewer, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return nil, nil, errors.New("gatekeeper: the command has ended")
	}
	if t.viewer != nil {
		t.cutLocked(askViewReplaced)
	}
	t.viewer = &askViewer{out: make(chan []byte, askViewerBuffer)}
	return append([]byte(nil), t.replay...), t.viewer, nil
}

func (t *askTerm) detach(v *askViewer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.viewer == v {
		t.cutLocked(askViewDetached)
	}
}

func (t *askTerm) cutLocked(reason string) {
	t.viewer.reason = reason
	close(t.viewer.out)
	t.viewer = nil
}

// input writes keystrokes to the command, only from the viewer currently attached.
func (t *askTerm) input(v *askViewer, data []byte) error {
	if len(data) > askInputBytes {
		return errors.New("gatekeeper: input is too large")
	}
	t.mu.Lock()
	current := t.viewer == v && !t.ended
	t.mu.Unlock()
	if !current {
		return errors.New("gatekeeper: this view is no longer attached")
	}
	_, err := t.pty.Write(data)
	return err
}

func (t *askTerm) resize(v *askViewer, rows, cols int) error {
	if rows < 1 || cols < 1 || rows > askTermMax || cols > askTermMax {
		return errors.New("gatekeeper: terminal size out of range")
	}
	t.mu.Lock()
	current := t.viewer == v && !t.ended
	t.mu.Unlock()
	if !current {
		return errors.New("gatekeeper: this view is no longer attached")
	}
	return t.pty.Resize(rows, cols)
}

// end cuts off the viewer for good; later output is still recorded but reaches no one.
func (t *askTerm) end() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ended = true
	if t.viewer != nil {
		t.cutLocked(askViewEnded)
	}
}
