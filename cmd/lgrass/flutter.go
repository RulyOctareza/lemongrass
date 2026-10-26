// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/android"
	"github.com/faizalv/lemongrass/internal/flutter"
)

const flutterUsage = `usage: lgrass flutter <command>   (run inside a Flutter project)

  run [--device <serial>] [--flavor <name>] [--target <file>] [--wait 8m] [-- <flutter run args>]
                                     Builds and starts the app in debug mode on the phone and keeps it
                                     running in the background; with --flavor and no --target,
                                     lib/main_<flavor>.dart is used when it exists
  status                             The running app's state, VM service and log path
  reload | restart | stop            Hot reload, hot restart or stop the running app
  test [<path>...] [--name <regex>] [--device <serial>] [--flavor <name>] [--no-record] [--allow-permissions] [--all]
                                     Runs tests and prints a JSON summary of the failures (--all lists every
                                     test). Paths under integration_test/ run on the phone and record one
                                     clip per test; --allow-permissions taps Allow on Android permission
                                     dialogs during the run, since flutter test reinstalls the app each time
  report [--all]                     The last test run's summary again; --all lists passing tests too
  http [--failed] [--id <id>] [--limit 30]
                                     HTTP calls the running app made, newest first; --id shows one call
                                     with headers and bodies. Auth headers and cookies are redacted`

func cmdFlutter(args []string) {
	if err := runFlutter(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// splitPassthrough separates lgrass flags from the arguments after --, which go to flutter as is.
func splitPassthrough(args []string) ([]string, []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func runFlutter(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "_serve" {
		return flutterServe(args[1:])
	}
	own, passthrough := splitPassthrough(args)
	f, err := parseCLIFlags(own, "device", "flavor", "target", "wait", "name", "id", "limit")
	if err != nil {
		return err
	}
	if len(f.positional) == 0 {
		return errors.New(flutterUsage)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	project, err := flutter.FindProject(cwd)
	if err != nil {
		return err
	}
	switch command := f.positional[0]; command {
	case "run":
		return flutterRun(ctx, project, f, passthrough)
	case "status":
		return flutterStatus(project)
	case "reload", "restart", "stop":
		message, err := flutter.Control(project, command)
		if err != nil {
			return err
		}
		fmt.Println(message)
		return nil
	case "test":
		return flutterTest(ctx, project, f, passthrough)
	case "report":
		return flutterReport(project, f.switches["all"])
	case "http":
		return flutterHTTP(project, f)
	default:
		return fmt.Errorf("unknown flutter command %q\n\n%s", command, flutterUsage)
	}
}

// flavorArgs turns --flavor and --target into flutter's own flags, defaulting the target to
// lib/main_<flavor>.dart, the layout most flavored projects use.
func flavorArgs(project string, f cliFlags) []string {
	var out []string
	flavor := f.values["flavor"]
	if flavor != "" {
		out = append(out, "--flavor", flavor)
	}
	target := f.values["target"]
	if target == "" && flavor != "" {
		candidate := filepath.Join("lib", "main_"+flavor+".dart")
		if _, err := os.Stat(filepath.Join(project, candidate)); err == nil {
			target = candidate
		}
	}
	if target != "" {
		out = append(out, "--target", target)
	}
	return out
}

func flutterRun(ctx context.Context, project string, f cliFlags, passthrough []string) error {
	client, err := android.Open(ctx, f.values["device"])
	if err != nil {
		return err
	}
	wait, err := f.duration("wait", 8*time.Minute)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := append(flavorArgs(project, f), passthrough...)
	if err := flutter.Spawn(self, project, client.Serial, args); err != nil {
		return err
	}
	fmt.Printf("building and starting on %s: flutter run %s\n", client.Serial, strings.Join(args, " "))
	s, err := flutter.Wait(project, wait)
	if err != nil {
		return err
	}
	return printRunState(project, s)
}

func flutterStatus(project string) error {
	dir, err := flutter.StateDir(project)
	if err != nil {
		return err
	}
	s, err := flutter.ReadState(dir)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("lgrass: no app has been run from this project. Start one with lgrass flutter run")
		return nil
	}
	if err != nil {
		return err
	}
	if (s.Status == flutter.StatusBuilding || s.Status == flutter.StatusStarted) && !flutter.Alive(s) && s.PID > 0 {
		s.Status = flutter.StatusStopped
		s.Message = "the run supervisor is gone"
	}
	return printRunState(project, s)
}

func printRunState(project string, s flutter.RunState) error {
	dir, err := flutter.StateDir(project)
	if err != nil {
		return err
	}
	fmt.Printf("status: %s\n", s.Status)
	if s.Message != "" {
		fmt.Printf("message: %s\n", s.Message)
	}
	if s.Serial != "" {
		fmt.Printf("device: %s\n", s.Serial)
	}
	if s.VMService != "" {
		fmt.Printf("vm service: %s\n", s.VMService)
	}
	fmt.Printf("log: %s\n", flutter.LogPath(dir))
	switch s.Status {
	case flutter.StatusBuilding:
		fmt.Println("still building. Check again with lgrass flutter status")
	case flutter.StatusFailed:
		return errors.New("flutter: the run failed, the end of the log:\n" + tail(flutter.LogPath(dir), 25))
	}
	return nil
}

func tail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// flutterServe is the detached supervisor flutter.Spawn starts; it is not meant to be run by hand.
func flutterServe(args []string) error {
	own, passthrough := splitPassthrough(args)
	if len(own) != 2 {
		return errors.New("usage: lgrass flutter _serve <project> <serial> -- <flutter run args>")
	}
	return flutter.Serve(own[0], own[1], passthrough, flutter.EnableHTTPProfiling)
}
