// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	logDate   = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)
	splitFrom = regexp.MustCompile(`(?i)\bsplit\s+(from|out\s+of)\b`)
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

const (
	relatedTasksLimit = 10
	relatedHitsLimit  = 3
)

type relatedTask struct {
	slug  string
	label string
	hits  []string
}

// relatedTasks lists the other active tasks that mention this one, or that this one names, as advice: tasks link in
// too many ways (splits, citations, origins, work moved out) for a link to prove a dependency, so no task waits on
// another. Timing suggests the relationship, and a "split from" line is quoted as evidence; the model judges.
func relatedTasks(env Env, slug string, age taskAge) []relatedTask {
	scratchpad := filepath.Join(env.Root, "biblio", "scratchpad")
	entries, err := os.ReadDir(scratchpad)
	if err != nil {
		return nil
	}
	mentions := slugWord(slug)
	var related []relatedTask
	for _, e := range entries {
		other := e.Name()
		if !e.IsDir() || other == slug || other == "archive" {
			continue
		}
		theirs := mdLines(env.Root, filepath.Join(scratchpad, other), mentions)
		ours := mdLines(env.Root, filepath.Join(scratchpad, slug), regexp.MustCompile(regexp.QuoteMeta("scratchpad/"+other+"/")))
		if len(theirs) == 0 && len(ours) == 0 {
			continue
		}
		r := relatedTask{slug: other}
		switch compareAge(ageOf(env, other), age) {
		case 1:
			r.label = "newer, could be a child"
		case -1:
			r.label = "older, could be a parent"
		default:
			r.label = "same age"
		}
		if anyMatch(hitTexts(theirs), splitFrom) {
			r.label += "; it says it was split from this task"
		}
		if anyMatch(hitTexts(ours), splitFrom) {
			r.label += "; this task says it was split from it"
		}
		for _, h := range append(theirs, ours...) {
			if len(r.hits) == relatedHitsLimit {
				r.hits = append(r.hits, fmt.Sprintf("and %d more", len(theirs)+len(ours)-relatedHitsLimit))
				break
			}
			r.hits = append(r.hits, h.String())
		}
		related = append(related, r)
		if len(related) == relatedTasksLimit {
			break
		}
	}
	return related
}

func (r relatedTask) String() string {
	return fmt.Sprintf("`%s` (%s):\n     - %s", r.slug, r.label, strings.Join(r.hits, "\n     - "))
}

type lineHit struct {
	file string // relative to the project root
	line int
	text string
}

func (h lineHit) String() string {
	return fmt.Sprintf("`%s:%d`: %s", h.file, h.line, clipRunes(h.text, 100))
}

func hitTexts(hits []lineHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.text
	}
	return out
}

// mdLines finds the lines matching re in the markdown files under dir.
func mdLines(root, dir string, re *regexp.Regexp) []lineHit {
	var hits []lineHit
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(readText(p), "\n") {
			if re.MatchString(line) {
				hits = append(hits, lineHit{filepath.ToSlash(rel), i + 1, strings.TrimSpace(line)})
			}
		}
		return nil
	})
	return hits
}

// slugWord matches the slug as a whole name, so `lgrass-rester` does not match `lgrass-rester-revamp`.
func slugWord(slug string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(slug) + `([^A-Za-z0-9_-]|$)`)
}

func readText(p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(data)
}

func anyMatch(lines []string, re *regexp.Regexp) bool {
	for _, l := range lines {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}
