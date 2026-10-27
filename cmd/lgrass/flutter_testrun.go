// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/android"
	"github.com/faizalv/lemongrass/internal/flutter"
)

func flutterTest(ctx context.Context, project string, f cliFlags, passthrough []string) error {
	sdk, err := flutter.SDK(project)
	if err != nil {
		return err
	}
	paths := f.positional[1:]
	if len(paths) == 0 {
		paths = []string{"test"}
	}
	integration := false
	for _, p := range paths {
		rel := filepath.ToSlash(p)
		if abs, err := filepath.Abs(p); err == nil {
			if r, err := filepath.Rel(project, abs); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		if strings.HasPrefix(rel, "integration_test") {
			integration = true
		}
	}

	args := []string{"test", "--machine"}
	var serial string
	var device *android.Client
	if integration {
		client, err := android.Open(ctx, f.values["device"])
		if err != nil {
			return err
		}
		serial, device = client.Serial, client
		args = append(args, "--device-id", serial)
		args = append(args, flavorArgsForTest(project, f)...)
	}
	if name := f.values["name"]; name != "" {
		args = append(args, "--name", name)
	}
	args = append(args, passthrough...)
	args = append(args, paths...)

	started := time.Now()
	id, dir, err := flutter.NewRunDir(project, started)
	if err != nil {
		return err
	}
	run := flutter.TestRun{ID: id, Project: project, Command: append([]string{"flutter"}, args...), Device: serial, StartedAt: started}

	var recorder *flutter.Recorder
	if integration && !f.switches["no-record"] {
		recorder, run.Note = startTestRecording(ctx, serial, filepath.Join(dir, "run.mp4"))
	}

	stopWatch := func() int { return 0 }
	if device != nil && f.switches["allow-permissions"] {
		stopWatch = watchPermissionDialogs(ctx, device)
	}
	collector := flutter.NewCollector(project)
	exitErr := runMachine(sdk, project, args, collector)
	if allowed := stopWatch(); allowed > 0 {
		run.Note = strings.TrimSpace(run.Note + fmt.Sprintf(" allowed %d permission dialog(s).", allowed))
	}
	run.DurationMs = time.Since(started).Milliseconds()
	collector.Finish(&run, exitErr)

	if recorder != nil {
		if err := recorder.Stop(); err != nil {
			run.Note = err.Error()
		} else {
			run.Recording = filepath.Join(dir, "run.mp4")
			if ffmpeg, err := android.FindTool("ffmpeg"); err != nil {
				run.Note = "clips were not cut: " + err.Error()
			} else if errs := recorder.CutClips(ffmpeg, run.Tests); len(errs) > 0 {
				run.Note = errors.Join(errs...).Error()
			}
		}
	}

	if err := flutter.SaveRun(dir, run); err != nil {
		return err
	}
	printJSON(flutter.Summary(run, f.switches["all"]))
	if !run.Success {
		return fmt.Errorf("flutter: %d failed, %d passed. lgrass flutter report shows this run again", run.Failed, run.Passed)
	}
	return nil
}

// flavorArgsForTest passes --flavor to an integration run. The test file is the entrypoint, so
// the flavor's main file is not used as a target here.
func flavorArgsForTest(project string, f cliFlags) []string {
	if flavor := f.values["flavor"]; flavor != "" {
		return []string{"--flavor", flavor}
	}
	return nil
}

// watchPermissionDialogs taps Allow on Android permission dialogs until the returned stop is
// called. flutter test reinstalls the app on every run, so a grant made before the run does not
// survive, and a test cannot tap a native dialog itself.
func watchPermissionDialogs(ctx context.Context, device *android.Client) func() int {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan int)
	go func() {
		allowed := 0
		for {
			select {
			case <-ctx.Done():
				done <- allowed
				return
			case <-time.After(1500 * time.Millisecond):
			}
			// A dump fails while the screen keeps animating; the next round tries again.
			if ok, err := device.AllowPermissionDialog(ctx); err == nil && ok {
				allowed++
				fmt.Fprintln(os.Stderr, "allowed a permission dialog")
			}
		}
	}()
	return func() int {
		cancel()
		return <-done
	}
}

func startTestRecording(ctx context.Context, serial, path string) (*flutter.Recorder, string) {
	scrcpy, err := android.FindTool("scrcpy")
	if err != nil {
		return nil, "not recorded: " + err.Error()
	}
	rec, err := flutter.StartRecording(ctx, scrcpy, serial, path)
	if err != nil {
		return nil, "not recorded: " + err.Error()
	}
	return rec, ""
}

// runMachine runs flutter with stdout and stderr merged into the collector, and prints a line
// per finished test to stderr so a long run shows progress.
func runMachine(sdk, project string, args []string, collector *flutter.Collector) error {
	cmd := exec.Command(sdk, args...)
	cmd.Dir = project
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("flutter: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			collector.Feed(line, time.Now())
			reportProgress(line)
		}
	}()
	err := cmd.Wait()
	writer.Close()
	<-done
	return err
}

func reportProgress(line string) {
	var m struct {
		Type    string `json:"type"`
		Result  string `json:"result"`
		Hidden  bool   `json:"hidden"`
		Skipped bool   `json:"skipped"`
	}
	if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &m) != nil {
		return
	}
	if m.Type == "testDone" && !m.Hidden {
		status := m.Result
		if m.Skipped {
			status = "skipped"
		}
		fmt.Fprintf(os.Stderr, "test %s\n", status)
	}
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func flutterReport(project string, all bool) error {
	run, err := flutter.LastRun(project)
	if err != nil {
		return err
	}
	printJSON(flutter.Summary(run, all))
	return nil
}

func flutterHTTP(project string, f cliFlags) error {
	dir, err := flutter.StateDir(project)
	if err != nil {
		return err
	}
	s, err := flutter.ReadState(dir)
	if err != nil || s.VMService == "" || !flutter.Alive(s) {
		return errors.New("flutter: no app is running from this project. Start one with lgrass flutter run, HTTP calls are recorded from then on")
	}
	vm, err := flutter.DialVM(s.VMService)
	if err != nil {
		return err
	}
	if id := f.values["id"]; id != "" {
		call, err := vm.HTTPCallDetail(id)
		if err != nil {
			return err
		}
		fmt.Print(flutter.FormatHTTPDetail(call))
		return nil
	}
	limit := 30
	if raw, ok := f.values["limit"]; ok {
		if limit, err = strconv.Atoi(raw); err != nil || limit <= 0 {
			return fmt.Errorf("--limit %q is not a positive number", raw)
		}
	}
	calls, err := vm.HTTPProfile()
	if err != nil {
		return err
	}
	printed := 0
	for _, c := range calls {
		if f.switches["failed"] && !c.Failed() {
			continue
		}
		if printed == 0 {
			fmt.Printf("%-12s %-6s %-5s %7s  %s\n", "ID", "METHOD", "CODE", "TIME", "URL")
		}
		fmt.Println(flutter.FormatHTTPLine(c))
		if printed++; printed == limit {
			break
		}
	}
	if printed == 0 {
		fmt.Println("lgrass: no matching HTTP calls yet.")
	} else {
		fmt.Println("lgrass flutter http --id <id> shows one call with headers and bodies.")
	}
	return nil
}
