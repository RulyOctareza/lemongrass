// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	logDate   = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)
	splitFrom = regexp.MustCompile(`(?i)\bsplit\s+(from|out\s+of)\b`)
	splitWord = regexp.MustCompile(`(?i)\bsplit\b`)
)

// taskAge is when a task started. The biblio history gives the moment; an activity log only gives the day, and only
// the day is compared when either side has no more.
type taskAge struct {
	t       time.Time
	precise bool
	known   bool
}

func (a taskAge) day() string {
	return a.t.Local().Format("2006-01-02")
}

// The biblio history keeps 90 days, so an older task falls back to the first date in its activity log.
func ageOf(env Env, slug string) taskAge {
	if env.TaskStarted != nil {
		if t, ok := env.TaskStarted(slug); ok {
			return taskAge{t: t, precise: true, known: true}
		}
	}
	data, err := os.ReadFile(filepath.Join(env.Root, "biblio", "scratchpad", slug, "activities.md"))
	if err != nil {
		return taskAge{}
	}
	if m := logDate.FindStringSubmatch(string(data)); m != nil {
		if t, err := time.ParseInLocation("2006-01-02", m[1], time.Local); err == nil {
			return taskAge{t: t, known: true}
		}
	}
	return taskAge{}
}

// compareAge is 1 when a is newer than b, -1 when older, and 0 when either is unknown or both fall on the same day
// without a moment to tell them apart.
func compareAge(a, b taskAge) int {
	if !a.known || !b.known {
		return 0
	}
	if a.precise && b.precise {
		switch {
		case a.t.After(b.t):
			return 1
		case a.t.Before(b.t):
			return -1
		}
		return 0
	}
	return strings.Compare(a.day(), b.day())
}

type childTask struct {
	slug   string
	reason string
}

// findChildren decides direction by age so two tasks can never wait on each other: of two tasks whose PRDs link, the
// newer is the child. Only a prd.md link counts, since a recon, notes or a brief citing a task is not a split. On a tie
// the "split from" wording decides, and without it the pair is left unrelated rather than blocking either close.
func findChildren(env Env, slug string, age taskAge) []childTask {
	scratchpad := filepath.Join(env.Root, "biblio", "scratchpad")
	entries, err := os.ReadDir(scratchpad)
	if err != nil {
		return nil
	}
	own := readText(filepath.Join(scratchpad, slug, "prd.md"))
	var children []childTask
	for _, e := range entries {
		other := e.Name()
		if !e.IsDir() || other == slug || other == "archive" {
			continue
		}
		ownLines := linesNaming(own, "scratchpad/"+other+"/")
		theirLines := linesNaming(readText(filepath.Join(scratchpad, other, "prd.md")), "scratchpad/"+slug+"/")
		if len(ownLines) == 0 && len(theirLines) == 0 {
			continue
		}
		link := "its PRD links here"
		if len(theirLines) == 0 {
			link = "this PRD links to it"
		}
		switch compareAge(ageOf(env, other), age) {
		case 1:
			children = append(children, childTask{other, "newer, " + link})
		case 0:
			switch {
			case anyMatch(ownLines, splitFrom):
			case anyMatch(theirLines, splitFrom):
				children = append(children, childTask{other, "same age, its PRD says it was split from this one"})
			case anyMatch(ownLines, splitWord):
				children = append(children, childTask{other, "same age, this PRD says it was split off"})
			}
		}
	}
	return children
}

func readText(p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(data)
}

func linesNaming(text, ref string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, ref) {
			out = append(out, line)
		}
	}
	return out
}

func anyMatch(lines []string, re *regexp.Regexp) bool {
	for _, l := range lines {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}
