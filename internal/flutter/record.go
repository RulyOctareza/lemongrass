// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package flutter

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// clipPadding widens each clip so the cut still holds the test's first and last frames, given
// keyframes a second apart and a host clock that only approximates the recording's start.
const clipPadding = time.Second

// Recorder records the phone's screen with a headless scrcpy while tests run.
type Recorder struct {
	cmd    *exec.Cmd
	path   string
	start  time.Time
	mu     sync.Mutex
	output []string
}

func (r *Recorder) lines() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.output, " ")
}

// StartRecording begins recording to path and returns once the first frame is in the file.
func StartRecording(ctx context.Context, scrcpy, serial, path string) (*Recorder, error) {
	cmd := exec.Command(scrcpy,
		"--serial="+serial,
		"--no-window",
		"--no-audio",
		"--no-control",
		"--max-size=1280",
		// One keyframe a second keeps per-test cuts accurate without re-encoding.
		"--video-codec-options=i-frame-interval:int=1",
		"--record="+path,
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("flutter: starting the recording: %w", err)
	}
	r := &Recorder{cmd: cmd, path: path}
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			r.mu.Lock()
			r.output = append(r.output, scanner.Text())
			r.mu.Unlock()
		}
	}()
	// scrcpy's own log is block-buffered on a pipe, so the start shows as the file growing:
	// the mp4 header is written when the first frame arrives.
	deadline := time.After(15 * time.Second)
	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			r.start = time.Now()
			return r, nil
		}
		select {
		case <-exited:
			return nil, fmt.Errorf("flutter: the recording stopped before it began: %s", r.lines())
		case <-deadline:
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("flutter: the recording did not start: %s", r.lines())
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Stop ends the recording; scrcpy finishes the mp4 on SIGINT.
func (r *Recorder) Stop() error {
	_ = r.cmd.Process.Signal(syscall.SIGINT)
	done := make(chan error, 1)
	go func() { done <- r.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = r.cmd.Process.Kill()
		return errors.New("flutter: the recording did not stop in time, the file may be incomplete")
	}
	if info, err := os.Stat(r.path); err != nil || info.Size() == 0 {
		return errors.New("flutter: the recording is empty")
	}
	return nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// ClipName is a file name for a test's clip.
func ClipName(index int, name string) string {
	slug := strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-")
	if len(slug) > 60 {
		slug = slug[:60]
	}
	return fmt.Sprintf("%02d-%s.mp4", index, slug)
}

// ClipWindow is where a test sits in a recording that began at recStart.
func ClipWindow(recStart, testStart, testEnd time.Time) (time.Duration, time.Duration) {
	from := testStart.Sub(recStart) - clipPadding
	if from < 0 {
		from = 0
	}
	to := testEnd.Sub(recStart) + clipPadding
	return from, to - from
}

// CutClips writes one clip per test that ran, next to the recording.
func (r *Recorder) CutClips(ffmpeg string, tests []TestResult) []error {
	var errs []error
	for i := range tests {
		t := &tests[i]
		if t.Start.IsZero() || t.End.IsZero() || t.Status == TestSkipped {
			continue
		}
		from, length := ClipWindow(r.start, t.Start, t.End)
		clip := filepath.Join(filepath.Dir(r.path), ClipName(i+1, t.Name))
		cmd := exec.Command(ffmpeg, "-y", "-loglevel", "error",
			"-ss", fmt.Sprintf("%.3f", from.Seconds()), "-i", r.path,
			"-t", fmt.Sprintf("%.3f", length.Seconds()), "-c", "copy", clip)
		if out, err := cmd.CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("flutter: cutting the clip of %q: %s", t.Name, strings.TrimSpace(string(out))))
			continue
		}
		t.Clip = clip
	}
	return errs
}
