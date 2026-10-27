// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
)

func TestParseResterArgsOutFlags(t *testing.T) {
	cmd, err := parseResterArgs([]string{"abc123", "get", "/export", "--out", "/tmp/x.bin", "--confirm"})
	if err != nil || cmd.out != "/tmp/x.bin" || !cmd.confirm {
		t.Fatalf("cmd = %+v, err = %v, want --out and --confirm parsed", cmd, err)
	}
	cmd, err = parseResterArgs([]string{"abc123", "post", "/export", "--out=/tmp/y.bin", "--body", "{}"})
	if err != nil || cmd.out != "/tmp/y.bin" || cmd.confirm || cmd.body != "{}" {
		t.Fatalf("cmd = %+v, err = %v, want --out=value together with a body", cmd, err)
	}

	bad := map[string][]string{
		"confirm without out":  {"abc123", "get", "/export", "--confirm"},
		"out on head":          {"abc123", "head", "/export", "--out", "/tmp/x"},
		"out on options":       {"abc123", "options", "/export", "--out", "/tmp/x"},
		"out on info":          {"abc123", "info", "--out", "/tmp/x"},
		"confirm on users":     {"abc123", "users", "--confirm"},
		"out on flush":         {"abc123", "flush", "--out", "/tmp/x"},
		"out without a target": {"abc123", "get", "/export", "--out"},
	}
	for name, args := range bad {
		if _, err := parseResterArgs(args); err == nil {
			t.Errorf("%s: parseResterArgs accepted %v", name, args)
		}
	}
}

func TestCheckOutTarget(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "have.bin")
	os.WriteFile(existing, []byte("old"), 0o644)

	if got, err := checkOutTarget(filepath.Join(dir, "new.bin"), false); err != nil || got.dir || got.path != filepath.Join(dir, "new.bin") {
		t.Errorf("new file: %+v, %v, want an absolute file target", got, err)
	}
	if got, err := checkOutTarget(dir, false); err != nil || !got.dir {
		t.Errorf("directory: %+v, %v, want a directory target", got, err)
	}
	if _, err := checkOutTarget(existing, false); err == nil || !strings.Contains(err.Error(), "--confirm") || !strings.Contains(err.Error(), "nothing was requested") {
		t.Errorf("existing file without confirm: err = %v, want a refusal naming --confirm", err)
	}
	if got, err := checkOutTarget(existing, true); err != nil || got.dir {
		t.Errorf("existing file with confirm: %+v, %v, want it accepted", got, err)
	}
	if _, err := checkOutTarget(filepath.Join(dir, "missing", "x.bin"), false); err == nil {
		t.Error("a file in a missing directory was accepted")
	}
	if _, err := checkOutTarget(filepath.Join(dir, "missing")+"/", false); err == nil {
		t.Error("a missing directory with a trailing slash was accepted")
	}
	if _, err := checkOutTarget(filepath.Join(existing, "inside"), false); err == nil {
		t.Error("a path below a regular file was accepted")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, downloadTempPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("checking targets left %v behind", leftovers)
	}
}

func streamOf(body io.Reader, name string, length int64) gatekeeper.DownloadResult {
	return gatekeeper.DownloadResult{
		Stream: &gatekeeper.StreamMeta{Status: 200, URL: "http://x/export", Name: name, Length: length},
		Body:   io.NopCloser(body),
	}
}

func noTemps(t *testing.T, dir string) {
	t.Helper()
	if leftovers, _ := filepath.Glob(filepath.Join(dir, downloadTempPrefix+"*")); len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestSaveDownloadWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	res, err := saveDownload(streamOf(strings.NewReader("hello"), "", 5), downloadTarget{path: target}, false)
	if err != nil {
		t.Fatalf("saveDownload: %v", err)
	}
	if res.SavedTo != target || res.Size != 5 || res.Status != 200 || res.URL != "http://x/export" {
		t.Errorf("result = %+v, want saved_to %s, size 5", res, target)
	}
	if got, _ := os.ReadFile(target); string(got) != "hello" {
		t.Errorf("file = %q, want hello", got)
	}
	if info, _ := os.Stat(target); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
	noTemps(t, dir)
}

func TestSaveDownloadReplacesOnlyWithConfirm(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	os.WriteFile(target, []byte("old"), 0o600)

	_, err := saveDownload(streamOf(strings.NewReader("new"), "", -1), downloadTarget{path: target}, false)
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("err = %v, want a refusal naming --confirm", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Errorf("file = %q, want it untouched", got)
	}

	if _, err := saveDownload(streamOf(strings.NewReader("new"), "", -1), downloadTarget{path: target}, true); err != nil {
		t.Fatalf("with confirm: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("file = %q, want it replaced", got)
	}
	noTemps(t, dir)
}

func TestSaveDownloadFailureLeavesTheExistingFileUntouched(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	os.WriteFile(target, []byte("precious"), 0o644)

	cut := io.MultiReader(strings.NewReader("part"), errAfter{errors.New("connection lost")})
	_, err := saveDownload(streamOf(cut, "", -1), downloadTarget{path: target}, true)
	if err == nil || !strings.Contains(err.Error(), "connection lost") || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("err = %v, want the failure reported", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "precious" {
		t.Errorf("file = %q, want it untouched", got)
	}
	noTemps(t, dir)
}

func TestSaveDownloadSizeMismatchSavesNothing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	_, err := saveDownload(streamOf(strings.NewReader("abc"), "", 10), downloadTarget{path: target}, false)
	if err == nil || !strings.Contains(err.Error(), "declared 10") {
		t.Fatalf("err = %v, want a size mismatch", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("a file was left at the target")
	}
	noTemps(t, dir)
}

func TestSaveDownloadIntoADirectoryUsesTheServerName(t *testing.T) {
	dir := t.TempDir()
	res, err := saveDownload(streamOf(strings.NewReader("x"), "report.csv", 1), downloadTarget{path: dir, dir: true}, false)
	if err != nil || res.SavedTo != filepath.Join(dir, "report.csv") {
		t.Fatalf("result = %+v, err = %v, want report.csv in the directory", res, err)
	}
	res, err = saveDownload(streamOf(strings.NewReader("x"), "", 1), downloadTarget{path: dir, dir: true}, false)
	if err != nil || res.SavedTo != filepath.Join(dir, defaultDownloadName) {
		t.Fatalf("result = %+v, err = %v, want the fallback name", res, err)
	}

	_, err = saveDownload(streamOf(strings.NewReader("y"), "report.csv", 1), downloadTarget{path: dir, dir: true}, false)
	if err == nil || !strings.Contains(err.Error(), "--confirm") || !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("err = %v, want the existing name refused after the headers", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "report.csv")); string(got) != "x" {
		t.Errorf("report.csv = %q, want it untouched", got)
	}

	os.Mkdir(filepath.Join(dir, "taken"), 0o755)
	if _, err := saveDownload(streamOf(bytes.NewReader([]byte("z")), "taken", 1), downloadTarget{path: dir, dir: true}, true); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("err = %v, want a directory in the way refused even with --confirm", err)
	}
	noTemps(t, dir)
}

type errAfter struct{ err error }

func (e errAfter) Read([]byte) (int, error) { return 0, e.err }
