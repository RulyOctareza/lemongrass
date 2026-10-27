// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package flutter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Test outcomes in a TestResult.
const (
	TestPassed  = "passed"
	TestFailed  = "failed"
	TestSkipped = "skipped"
)

const (
	maxOutputLines = 40
	// maxOutputBytes caps a failure's printed output; one Flutter exception dump can run to
	// several kilobytes, and the summary is read by an agent whose context is the cost.
	maxOutputBytes = 3000
)

// TestResult is one test of a run.
type TestResult struct {
	Name       string `json:"name"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Stack      string `json:"stack,omitempty"`
	// Output holds what the test printed, kept only for a failure.
	Output []string `json:"output,omitempty"`
	Clip   string   `json:"clip,omitempty"`
	// Start and End are host times, used to cut the test's clip out of a run recording.
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// TestRun is the summary lgrass flutter test prints and keeps for lgrass flutter report.
type TestRun struct {
	ID         string       `json:"id"`
	Project    string       `json:"project"`
	Command    []string     `json:"command"`
	Device     string       `json:"device,omitempty"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMs int64        `json:"duration_ms"`
	Success    bool         `json:"success"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Skipped    int          `json:"skipped"`
	Recording  string       `json:"recording,omitempty"`
	Note       string       `json:"note,omitempty"`
	Tests      []TestResult `json:"tests"`
	// ToolOutput is the end of the flutter tool's own output, kept when the run broke before
	// reporting any test, such as a build failure.
	ToolOutput []string `json:"tool_output,omitempty"`
}

type machineTest struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	SuiteID int    `json:"suiteID"`
	Line    *int   `json:"line"`
	// RootLine is set when the test was declared through a helper such as testWidgets; it
	// points into the test file, while Line points into the helper.
	RootLine *int   `json:"root_line"`
	RootURL  string `json:"root_url"`
	URL      string `json:"url"`
}

type machineLine struct {
	Type  string      `json:"type"`
	Time  int64       `json:"time"`
	Test  machineTest `json:"test"`
	Suite struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
	} `json:"suite"`
	TestID     int    `json:"testID"`
	Result     string `json:"result"`
	Hidden     bool   `json:"hidden"`
	Skipped    bool   `json:"skipped"`
	Error      string `json:"error"`
	StackTrace string `json:"stackTrace"`
	Message    string `json:"message"`
	Success    *bool  `json:"success"`
}

type liveTest struct {
	result TestResult
	hidden bool
	output []string
	// lineKnown is false when the reporter gave only the line of a helper such as testWidgets.
	lineKnown bool
}

// Collector turns flutter test --machine lines into a TestRun.
type Collector struct {
	project string
	suites  map[int]string
	tests   map[int]*liveTest
	order   []int
	tool    []string
	done    *bool
}

func NewCollector(project string) *Collector {
	return &Collector{project: project, suites: map[int]string{}, tests: map[int]*liveTest{}}
}

// Feed reads one output line; now is when the host saw it.
func (c *Collector) Feed(line string, now time.Time) {
	var m machineLine
	if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &m) != nil || m.Type == "" {
		c.tool = append(c.tool, line)
		if len(c.tool) > maxOutputLines {
			c.tool = c.tool[1:]
		}
		return
	}
	switch m.Type {
	case "suite":
		c.suites[m.Suite.ID] = c.relative(m.Suite.Path)
	case "testStart":
		t := &liveTest{result: TestResult{Name: m.Test.Name, File: c.suites[m.Test.SuiteID], Start: now}}
		switch {
		case m.Test.RootLine != nil:
			t.result.Line, t.lineKnown = *m.Test.RootLine, true
		case m.Test.Line != nil && !strings.HasPrefix(m.Test.URL, "package:"):
			t.result.Line, t.lineKnown = *m.Test.Line, true
		}
		c.tests[m.Test.ID] = t
		c.order = append(c.order, m.Test.ID)
	case "print":
		if t := c.tests[m.TestID]; t != nil && len(t.output) < maxOutputLines {
			used := 0
			for _, o := range t.output {
				used += len(o)
			}
			if left := maxOutputBytes - used; left > 0 {
				msg := m.Message
				if len(msg) > left {
					msg = msg[:left] + " ..."
				}
				t.output = append(t.output, msg)
			}
		}
	case "error":
		if t := c.tests[m.TestID]; t != nil {
			// An error after testDone is async work the test left running; flutter counts the
			// test as failed, so the summary does too.
			if t.result.Status == TestPassed {
				t.result.Status = TestFailed
				t.result.Output = t.output
			}
			if t.result.Error != "" {
				t.result.Error += "\n"
			}
			t.result.Error += strings.TrimSpace(m.Error)
			if t.result.Stack == "" {
				t.result.Stack = trimStack(m.StackTrace)
			}
		}
	case "testDone":
		t := c.tests[m.TestID]
		if t == nil {
			return
		}
		t.result.End = now
		t.result.DurationMs = now.Sub(t.result.Start).Milliseconds()
		t.hidden = m.Hidden
		switch {
		case m.Skipped:
			t.result.Status = TestSkipped
		case m.Result == "success":
			t.result.Status = TestPassed
		default:
			t.result.Status = TestFailed
			t.result.Output = t.output
			if !t.lineKnown {
				t.result.Line = lineInFile(t.result.File, t.result.Stack, t.output)
			}
		}
	case "done":
		c.done = m.Success
	}
}

// Finish builds the run. exitErr is how the flutter process ended.
func (c *Collector) Finish(run *TestRun, exitErr error) {
	loadFailed := false
	for _, id := range c.order {
		t := c.tests[id]
		// A hidden test is the loader of a file; it matters only when the file failed to load.
		if t.hidden && t.result.Status != TestFailed {
			continue
		}
		if t.hidden {
			loadFailed = true
		}
		if t.result.Status == "" {
			t.result.Status = TestFailed
			t.result.Error = "the test did not finish"
		}
		if t.result.Status == TestFailed {
			t.result.Output = t.output
			if !t.lineKnown {
				t.result.Line = lineInFile(t.result.File, t.result.Stack, t.output)
			}
		}
		run.Tests = append(run.Tests, t.result)
		switch t.result.Status {
		case TestPassed:
			run.Passed++
		case TestSkipped:
			run.Skipped++
		default:
			run.Failed++
		}
	}
	run.Success = exitErr == nil && run.Failed == 0 && (c.done == nil || *c.done)
	// A file that failed to load, such as one that does not compile, says why only in the
	// tool's own output.
	if len(run.Tests) == 0 || (!run.Success && run.Failed == 0) || loadFailed {
		run.Success = false
		run.ToolOutput = c.tool
	}
}

// lineInFile finds the outermost stack frame in the test's own file, the one closest to the
// test body rather than a helper it called. A device run reports no root_line, so this is the
// only pointer into the test file it has.
func lineInFile(file, stack string, output []string) int {
	if file == "" {
		return 0
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(filepath.ToSlash(file)) + `:(\d+):\d+`)
	line := 0
	for _, text := range append([]string{stack}, output...) {
		for _, m := range pattern.FindAllStringSubmatch(text, -1) {
			line, _ = strconv.Atoi(m[1])
		}
	}
	return line
}

func (c *Collector) relative(path string) string {
	if rel, err := filepath.Rel(c.project, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// trimStack keeps the frames of the project's own code and the first few others.
func trimStack(stack string) string {
	lines := strings.Split(strings.TrimSpace(stack), "\n")
	var out []string
	for _, l := range lines {
		if len(out) >= 12 {
			break
		}
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Summary is the run as lgrass flutter test prints it: failures always, passing tests only
// when all is set.
func Summary(run TestRun, all bool) TestRun {
	out := run
	out.Tests = nil
	for _, t := range run.Tests {
		if all || t.Status == TestFailed {
			out.Tests = append(out.Tests, t)
		}
	}
	if out.Tests == nil {
		out.Tests = []TestResult{}
	}
	return out
}

func runsDir(project string) (string, error) {
	dir, err := StateDir(project)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tests"), nil
}

// NewRunDir makes the directory for one test run and returns its id and path.
func NewRunDir(project string, now time.Time) (string, string, error) {
	base, err := runsDir(project)
	if err != nil {
		return "", "", err
	}
	id := now.Format("20060102-150405")
	dir := filepath.Join(base, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("flutter: %w", err)
	}
	return id, dir, nil
}

// SaveRun writes result.json into the run's directory.
func SaveRun(dir string, run TestRun) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644)
}

// LastRun reads the newest saved run.
func LastRun(project string) (TestRun, error) {
	var run TestRun
	base, err := runsDir(project)
	if err != nil {
		return run, err
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) == 0 {
		return run, fmt.Errorf("flutter: no test run has been recorded for this project yet")
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i := len(names) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(base, names[i], "result.json"))
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &run); err != nil {
			return run, fmt.Errorf("flutter: reading %s: %w", names[i], err)
		}
		return run, nil
	}
	return run, fmt.Errorf("flutter: no finished test run has been recorded for this project yet")
}
