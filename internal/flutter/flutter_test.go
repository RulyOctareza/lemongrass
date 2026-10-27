// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package flutter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func feedAll(c *Collector, lines []string) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, l := range lines {
		c.Feed(l, base.Add(time.Duration(i)*time.Second))
	}
}

func TestCollectorReportsPassFailSkipAndHidesLoaders(t *testing.T) {
	c := NewCollector("/work/shop")
	feedAll(c, []string{
		`{"protocolVersion":"0.1.1","runnerVersion":null,"pid":1,"type":"start","time":0}`,
		`{"suite":{"id":0,"platform":"vm","path":"/work/shop/test/cart_test.dart"},"type":"suite","time":0}`,
		`{"test":{"id":1,"name":"loading /work/shop/test/cart_test.dart","suiteID":0,"groupIDs":[],"line":null,"url":null},"type":"testStart","time":1}`,
		`{"testID":1,"result":"success","skipped":false,"hidden":true,"type":"testDone","time":2}`,
		`{"test":{"id":3,"name":"cart adds an item","suiteID":0,"groupIDs":[2],"line":12,"url":"file:///work/shop/test/cart_test.dart"},"type":"testStart","time":3}`,
		`{"testID":3,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":4}`,
		`{"test":{"id":4,"name":"cart totals","suiteID":0,"groupIDs":[2],"line":174,"url":"package:flutter_test/src/widget_tester.dart","root_line":20,"root_url":"file:///work/shop/test/cart_test.dart"},"type":"testStart","time":5}`,
		`{"testID":4,"messageType":"print","message":"total is 3","type":"print","time":6}`,
		`{"testID":4,"error":"Expected: <4>\n  Actual: <3>","stackTrace":"package:shop/cart.dart 10:3\n\npackage:test_api expect","isFailure":true,"type":"error","time":7}`,
		`{"testID":4,"result":"failure","skipped":false,"hidden":false,"type":"testDone","time":8}`,
		`{"test":{"id":5,"name":"cart later","suiteID":0,"groupIDs":[2],"line":30},"type":"testStart","time":9}`,
		`{"testID":5,"result":"success","skipped":true,"hidden":false,"type":"testDone","time":10}`,
		`{"success":false,"type":"done","time":11}`,
	})
	var run TestRun
	c.Finish(&run, errors.New("exit status 1"))
	if run.Passed != 1 || run.Failed != 1 || run.Skipped != 1 || run.Success {
		t.Fatalf("counts = %+v", run)
	}
	if len(run.Tests) != 3 {
		t.Fatalf("tests = %+v", run.Tests)
	}
	failed := run.Tests[1]
	if failed.File != "test/cart_test.dart" || failed.Line != 20 || !strings.Contains(failed.Error, "Actual: <3>") {
		t.Fatalf("failed = %+v", failed)
	}
	if len(failed.Output) != 1 || failed.Output[0] != "total is 3" || failed.DurationMs != 3000 {
		t.Fatalf("failed output/duration = %+v", failed)
	}
	if run.Tests[0].Output != nil {
		t.Fatalf("a passing test kept its output")
	}
}

func TestCollectorTakesTheLineFromTheStackOnADeviceRun(t *testing.T) {
	c := NewCollector("/work/shop")
	feedAll(c, []string{
		`{"suite":{"id":0,"platform":"android","path":"/work/shop/integration_test/flow_test.dart"},"type":"suite","time":0}`,
		`{"test":{"id":2,"name":"flow","suiteID":0,"groupIDs":[],"line":174,"url":"package:flutter_test/src/widget_tester.dart"},"type":"testStart","time":1}`,
		`{"testID":2,"messageType":"print","message":"#0      waitFor (file:///work/shop/integration_test/flow_test.dart:69:3)","type":"print","time":2}`,
		`{"testID":2,"messageType":"print","message":"#1      main.<anonymous closure> (file:///work/shop/integration_test/flow_test.dart:36:5)","type":"print","time":2}`,
		`{"testID":2,"error":"Test failed.","stackTrace":"","isFailure":true,"type":"error","time":3}`,
		`{"testID":2,"result":"error","skipped":false,"hidden":false,"type":"testDone","time":4}`,
	})
	var run TestRun
	c.Finish(&run, errors.New("exit status 1"))
	if got := run.Tests[0].Line; got != 36 {
		t.Fatalf("line = %d, want 36", got)
	}
}

func TestCollectorFailsATestThatErrorsAfterItFinished(t *testing.T) {
	c := NewCollector("/work/shop")
	feedAll(c, []string{
		`{"suite":{"id":0,"platform":"android","path":"/work/shop/integration_test/flow_test.dart"},"type":"suite","time":0}`,
		`{"test":{"id":2,"name":"flow","suiteID":0,"groupIDs":[],"line":174,"url":"package:flutter_test/src/widget_tester.dart"},"type":"testStart","time":1}`,
		`{"testID":2,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":2}`,
		`{"testID":2,"messageType":"print","message":"StateError: Bad state: late call","type":"print","time":3}`,
		`{"testID":2,"error":"This test failed after it had already completed.","stackTrace":"","isFailure":false,"type":"error","time":4}`,
		`{"success":false,"type":"done","time":5}`,
	})
	var run TestRun
	c.Finish(&run, nil)
	if run.Passed != 0 || run.Failed != 1 || run.Success {
		t.Fatalf("run = %+v", run)
	}
	if out := run.Tests[0].Output; len(out) != 1 || !strings.Contains(out[0], "late call") {
		t.Fatalf("output = %q", out)
	}
}

func TestCollectorKeepsALoadFailureAndTheToolOutput(t *testing.T) {
	c := NewCollector("/work/shop")
	feedAll(c, []string{
		`Running Gradle task 'assembleDebug'...`,
		`{"suite":{"id":0,"platform":"vm","path":"/work/shop/test/broken_test.dart"},"type":"suite","time":0}`,
		`{"test":{"id":1,"name":"loading /work/shop/test/broken_test.dart","suiteID":0,"groupIDs":[],"line":null},"type":"testStart","time":1}`,
		`{"testID":1,"error":"Error: Undefined name 'cart'.","stackTrace":"","isFailure":false,"type":"error","time":2}`,
		`{"testID":1,"result":"error","skipped":false,"hidden":true,"type":"testDone","time":3}`,
	})
	var run TestRun
	c.Finish(&run, errors.New("exit status 1"))
	if run.Failed != 1 || !strings.Contains(run.Tests[0].Error, "Undefined name") {
		t.Fatalf("run = %+v", run)
	}
	if len(run.ToolOutput) != 1 || !strings.Contains(run.ToolOutput[0], "Gradle") {
		t.Fatalf("tool output = %q", run.ToolOutput)
	}
}

func TestCollectorWithNoTestsKeepsTheToolOutput(t *testing.T) {
	c := NewCollector("/work/shop")
	feedAll(c, []string{"FAILURE: Build failed with an exception.", "* What went wrong:"})
	var run TestRun
	c.Finish(&run, errors.New("exit status 1"))
	if run.Success || len(run.ToolOutput) != 2 {
		t.Fatalf("run = %+v", run)
	}
}

func TestSummaryKeepsFailuresUnlessAll(t *testing.T) {
	run := TestRun{Tests: []TestResult{{Name: "a", Status: TestPassed}, {Name: "b", Status: TestFailed}}}
	if got := Summary(run, false).Tests; len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("summary = %+v", got)
	}
	if got := Summary(run, true).Tests; len(got) != 2 {
		t.Fatalf("summary all = %+v", got)
	}
	if got := Summary(TestRun{}, false).Tests; got == nil {
		t.Fatalf("an empty summary must print [] not null")
	}
}

func TestClipWindowPadsAndClampsAtZero(t *testing.T) {
	rec := time.Unix(100, 0)
	from, length := ClipWindow(rec, rec.Add(5*time.Second), rec.Add(8*time.Second))
	if from != 4*time.Second || length != 5*time.Second {
		t.Fatalf("window = %s %s", from, length)
	}
	from, _ = ClipWindow(rec, rec.Add(200*time.Millisecond), rec.Add(time.Second))
	if from != 0 {
		t.Fatalf("from = %s", from)
	}
}

func TestClipName(t *testing.T) {
	if got := ClipName(3, "login: shows an error / on 401"); got != "03-login-shows-an-error-on-401.mp4" {
		t.Fatalf("ClipName = %s", got)
	}
}

func TestDialVMTurnsTheWebsocketURIIntoHTTP(t *testing.T) {
	vm, err := DialVM("ws://127.0.0.1:59110/-Ap2=/ws")
	if err != nil || vm.base != "http://127.0.0.1:59110/-Ap2=/" {
		t.Fatalf("base = %v %v", vm, err)
	}
}

func TestDetailRedactsCredentials(t *testing.T) {
	call := HTTPCall{
		Method:   "POST",
		URI:      "https://api.example.test/login",
		Request:  &profileHalf{Headers: map[string][]string{"Authorization": {"Bearer abc"}, "accept": {"json"}}},
		Response: &profileHalf{StatusCode: 200, Headers: map[string][]string{"set-cookie": {"sid=1"}}},
		ReqBody:  []byte(`{"email":"a@example.test","password":"hunter2"}`),
		RespBody: []byte(`{"data":{"accessToken":"xyz","user":{"name":"A"}}}`),
	}
	out := FormatHTTPDetail(call)
	for _, secret := range []string{"Bearer abc", "sid=1", "hunter2", "xyz"} {
		if strings.Contains(out, secret) {
			t.Fatalf("detail leaks %q:\n%s", secret, out)
		}
	}
	for _, kept := range []string{"a@example.test", `"name": "A"`, "accept: json"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("detail lost %q:\n%s", kept, out)
		}
	}
}

func TestStatusAndFailed(t *testing.T) {
	cases := []struct {
		call   HTTPCall
		status string
		failed bool
	}{
		{HTTPCall{EndTime: 1, Response: &profileHalf{StatusCode: 200}}, "200", false},
		{HTTPCall{EndTime: 1, Response: &profileHalf{StatusCode: 401}}, "401", true},
		{HTTPCall{EndTime: 1, Request: &profileHalf{Error: "SocketException"}}, "ERROR SocketException", true},
		{HTTPCall{}, "pending", false},
	}
	for _, c := range cases {
		if c.call.Status() != c.status || c.call.Failed() != c.failed {
			t.Errorf("%+v: status %q failed %v", c.call, c.call.Status(), c.call.Failed())
		}
	}
}

func TestFindProjectWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte("name: shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "lib", "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindProject(deep)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if gotReal, _ := filepath.EvalSymlinks(got); gotReal != want {
		t.Fatalf("FindProject = %s, want %s", got, root)
	}
}

func TestSDKPrefersTheFVMPin(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, ".fvm", "flutter_sdk", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "flutter"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := SDK(root); err != nil || got != filepath.Join(bin, "flutter") {
		t.Fatalf("SDK = %s %v", got, err)
	}
}
