// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A PRD above either limit sends its task to the full close.
const (
	lightPlansLimit = 6
	lightWordsLimit = 500
)

var (
	whiteboardRow = regexp.MustCompile(`^\d+\.\s+(\S+)\s.*->\s*(\S+)\s*$`)
	plansItem     = regexp.MustCompile(`^\s*[-*]\s+\\?\[([ xX])\\?\]\s*(.*)$`)
)

type closeFacts struct {
	slug        string
	kind        string // build, recon, debug or notes, from the reserved files the task holds
	handover    string
	age         taskAge
	children    []childTask
	plans       int
	words       int
	open        []string
	noRootCause bool
	footprint   footprint
}

func (f closeFacts) full() bool {
	return f.handover != "" || len(f.children) > 0 || f.plans > lightPlansLimit || f.words > lightWordsLimit ||
		f.footprint.newFiles() > 0 || len(f.footprint.files) > lightFilesLimit
}

// A step is one fragment followed by the lists the tool already gathered for it.
type closeStep struct {
	file     string
	sections []closeList
}

type closeList struct {
	lead  string
	items []string
}

func taskClose(env Env, args []string) (string, error) {
	slug, err := parseSlug(args, "lgrass how task close <slug>")
	if err != nil {
		return "", err
	}
	facts, err := inspectTask(env, slug)
	if err != nil {
		return "", err
	}
	if len(facts.open) > 0 || facts.noRootCause {
		return unfinishedNotice(facts), nil
	}
	root := env.Root
	dir := filepath.Join(root, "biblio", "scratchpad", slug)
	vars := map[string]string{"slug": slug, "handover": facts.handover}

	var childLines []string
	for _, c := range facts.children {
		childLines = append(childLines, fmt.Sprintf("`%s` (%s)", c.slug, c.reason))
	}
	var fileLines []string
	for i, f := range facts.footprint.files {
		if i == footprintListLimit {
			fileLines = append(fileLines, fmt.Sprintf("and %d more", len(facts.footprint.files)-i))
			break
		}
		line := "`" + f.path + "`"
		if f.state != "" {
			line += " " + f.state
		}
		fileLines = append(fileLines, line)
	}
	filesLead := "Files the task's documents name:"
	if facts.footprint.fromEdits {
		filesLead = "Files edited by the sessions that wrote this task's documents. A session that also worked other tasks brings their files too; leave those out:"
	}
	var chapterLines []string
	for _, c := range relatedChapters(root, facts.footprint.paths()) {
		chapterLines = append(chapterLines, c.String())
	}
	docs := readText(filepath.Join(dir, "prd.md")) + "\n" + readText(filepath.Join(dir, "recon.md")) + "\n" + readText(filepath.Join(dir, "notes.md"))
	questions := closeList{"Lines that may hold an unsettled question; settle each, or leave it when it is not one:", matchingLines(docs, questionLine)}
	files := closeList{filesLead, fileLines}
	chapters := closeList{"Chapters that name these files. Check each against the code and correct a now-false fact in place:", chapterLines}
	stale := closeStep{"sweep.md", []closeList{{"References found before the archive; fix each to point at the archived path or drop it:", staleRefs(root, slug, facts.handover)}}}

	full := facts.full()
	doneFile, booksFile := facts.kind+"-done.md", facts.kind+"-books.md"
	if facts.kind == "build" {
		doneFile, booksFile = "light-done.md", "light-books.md"
		if full {
			doneFile, booksFile = "full-done.md", "full-books.md"
		}
	}
	books := closeStep{booksFile, []closeList{chapters}}

	var steps []closeStep
	if full {
		steps = append(steps, closeStep{doneFile, []closeList{questions}}, closeStep{"full-git.md", []closeList{files}})
		if len(facts.children) > 0 {
			steps = append(steps, closeStep{"full-children.md", []closeList{{"", childLines}}})
		}
		steps = append(steps,
			closeStep{"full-scope.md", []closeList{{"Written in the PRD:", matchingLines(readText(filepath.Join(dir, "prd.md")), scopeLine)}}},
			closeStep{file: "full-docs.md"},
			books,
			closeStep{"full-laws.md", []closeList{{"Activity log lines that mention a correction:", matchingLines(readText(filepath.Join(dir, "activities.md")), correctionLine)}}},
			closeStep{file: "archive.md"})
		if facts.handover != "" {
			steps = append(steps, closeStep{file: "handover.md"})
		}
		steps = append(steps, stale, closeStep{file: "full-report.md"})
	} else {
		steps = []closeStep{{doneFile, []closeList{files, questions}}, {file: "light-log.md"}, books, {file: "archive.md"}, stale, {file: "light-report.md"}}
	}

	var b strings.Builder
	b.WriteString(closeHeadline(facts, full))
	b.WriteString("\n\n")
	b.WriteString(fragment("task/close/bar.md", vars))
	b.WriteString("\n\n## Steps\n\n")
	for i, step := range steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, fragment("task/close/"+step.file, vars))
		for _, list := range step.sections {
			if len(list.items) == 0 {
				continue
			}
			if list.lead != "" {
				fmt.Fprintf(&b, "   %s\n", list.lead)
			}
			for _, item := range list.items {
				fmt.Fprintf(&b, "   - %s\n", item)
			}
		}
	}
	b.WriteString("\n## Books\n\n")
	b.WriteString(fragment("book/rules.md", vars))
	b.WriteString("\n")
	return b.String(), nil
}

// parseSlug accepts a task slug or a path to its directory.
func parseSlug(args []string, usage string) (string, error) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("how: usage: %s", usage)
	}
	slug := filepath.Base(strings.TrimRight(args[0], "/"))
	if slug == "." || slug == "archive" {
		return "", fmt.Errorf("how: usage: %s", usage)
	}
	return slug, nil
}

func inspectTask(env Env, slug string) (closeFacts, error) {
	dir := filepath.Join(env.Root, "biblio", "scratchpad", slug)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return closeFacts{}, fmt.Errorf("how: no active task directory biblio/scratchpad/%s", slug)
	}
	facts := closeFacts{slug: slug, kind: taskKind(dir), handover: findHandover(env.Root, slug)}
	facts.age = ageOf(env, slug)
	facts.children = findChildren(env, slug, facts.age)
	facts.plans, facts.words, facts.open = readPRD(filepath.Join(dir, "prd.md"))
	if debug := filepath.Join(dir, "debug.md"); exists(debug) && !rootCause.MatchString(readText(debug)) {
		facts.noRootCause = true
	}
	facts.footprint = taskFootprint(env, slug, facts.age)
	return facts, nil
}

// The first reserved file present names the kind, so a recon that grew a PRD closes as the build it became.
func taskKind(dir string) string {
	for _, k := range []struct{ file, kind string }{{"prd.md", "build"}, {"recon.md", "recon"}, {"debug.md", "debug"}} {
		if exists(filepath.Join(dir, k.file)) {
			return k.kind
		}
	}
	return "notes"
}

// A whiteboard row names the handover file; a file named after the slug counts without one.
func findHandover(root, slug string) string {
	dir := filepath.Join(root, "biblio", "handover")
	if data, err := os.ReadFile(filepath.Join(dir, "whiteboard.md")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if m := whiteboardRow.FindStringSubmatch(strings.TrimSpace(line)); m != nil && m[1] == slug {
				return m[2]
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, slug+".md")); err == nil {
		return slug + ".md"
	}
	return ""
}

func readPRD(prdPath string) (plans, words int, open []string) {
	data, err := os.ReadFile(prdPath)
	if err != nil {
		return 0, 0, nil
	}
	words = len(strings.Fields(string(data)))
	for _, line := range strings.Split(string(data), "\n") {
		m := plansItem.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		plans++
		if m[1] == " " {
			open = append(open, strings.TrimSpace(m[2]))
		}
	}
	return plans, words, open
}

func closeHeadline(f closeFacts, full bool) string {
	var reasons []string
	if f.handover != "" {
		reasons = append(reasons, "handover "+f.handover)
	} else {
		reasons = append(reasons, "no handover")
	}
	if len(f.children) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d child task(s)", len(f.children)))
	} else {
		reasons = append(reasons, "no child task")
	}
	reasons = append(reasons, fmt.Sprintf("%d PRD words", f.words), fmt.Sprintf("%d Plans item(s)", f.plans))
	switch {
	case len(f.footprint.files) == 0:
		reasons = append(reasons, "no code files found")
	case f.footprint.newFiles() > 0:
		reasons = append(reasons, fmt.Sprintf("%d code file(s), %d new", len(f.footprint.files), f.footprint.newFiles()))
	default:
		reasons = append(reasons, fmt.Sprintf("%d code file(s)", len(f.footprint.files)))
	}
	if f.age.known {
		reasons = append(reasons, "started "+f.age.day())
	}
	path := "Light close"
	if full {
		path = "Full close"
	}
	return fmt.Sprintf("%s for %s, a %s task: %s.", path, f.slug, f.kind, strings.Join(reasons, ", "))
}

func unfinishedNotice(f closeFacts) string {
	var b strings.Builder
	if len(f.open) > 0 {
		fmt.Fprintf(&b, "%s is not ready to close. These Plans items are unchecked:\n", f.slug)
		for _, item := range f.open {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		fmt.Fprintf(&b, "\nFinish each one, or mark it `[x]` if the code already shows it done, then run `lgrass how task close %s` again. If one cannot be finished, tell the user the task cannot close and why.\n", f.slug)
	}
	if f.noRootCause {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s is not ready to close: `biblio/scratchpad/%s/debug.md` names no root cause. Record the root cause, then run `lgrass how task close %s` again. If it was never found, tell the user the task cannot close and why.\n", f.slug, f.slug, f.slug)
	}
	return b.String()
}
