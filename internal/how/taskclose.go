// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package how

import (
	"fmt"
	"io/fs"
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
	slug     string
	handover string
	children []string
	plans    int
	words    int
	open     []string
}

func (f closeFacts) full() bool {
	return f.handover != "" || len(f.children) > 0 || f.plans > lightPlansLimit || f.words > lightWordsLimit
}

func taskClose(root string, args []string) (string, error) {
	slug, err := parseCloseArgs(args)
	if err != nil {
		return "", err
	}
	facts, err := inspectTask(root, slug)
	if err != nil {
		return "", err
	}
	if len(facts.open) > 0 {
		return unfinishedNotice(facts), nil
	}
	vars := map[string]string{"slug": slug, "handover": facts.handover, "children": strings.Join(facts.children, ", ")}

	var steps []string
	full := facts.full()
	if full {
		steps = append(steps, "full-done.md", "full-git.md")
		if len(facts.children) > 0 {
			steps = append(steps, "full-children.md")
		}
		steps = append(steps, "full-scope.md", "full-docs.md", "full-books.md", "full-laws.md", "archive.md")
		if facts.handover != "" {
			steps = append(steps, "handover.md")
		}
		steps = append(steps, "sweep.md", "full-report.md")
	} else {
		steps = []string{"light-done.md", "light-log.md", "light-books.md", "archive.md", "sweep.md", "light-report.md"}
	}

	var b strings.Builder
	b.WriteString(closeHeadline(facts, full))
	b.WriteString("\n\n")
	b.WriteString(fragment("task/close/bar.md", vars))
	b.WriteString("\n\n## Steps\n\n")
	for i, step := range steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, fragment("task/close/"+step, vars))
	}
	b.WriteString("\n## Books\n\n")
	b.WriteString(fragment("task/close/books.md", vars))
	b.WriteString("\n")
	return b.String(), nil
}

func parseCloseArgs(args []string) (string, error) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("how: usage: lgrass how task close <slug>")
	}
	slug := filepath.Base(strings.TrimRight(args[0], "/"))
	if slug == "." || slug == "archive" {
		return "", fmt.Errorf("how: usage: lgrass how task close <slug>")
	}
	return slug, nil
}

func inspectTask(root, slug string) (closeFacts, error) {
	scratchpad := filepath.Join(root, "biblio", "scratchpad")
	info, err := os.Stat(filepath.Join(scratchpad, slug))
	if err != nil || !info.IsDir() {
		return closeFacts{}, fmt.Errorf("how: no active task directory biblio/scratchpad/%s", slug)
	}
	facts := closeFacts{slug: slug, handover: findHandover(root, slug)}
	facts.children = findChildren(scratchpad, slug)
	facts.plans, facts.words, facts.open = readPRD(filepath.Join(scratchpad, slug, "prd.md"))
	return facts, nil
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

// A child is another active task with a file that references this task's directory.
func findChildren(scratchpad, slug string) []string {
	entries, err := os.ReadDir(scratchpad)
	if err != nil {
		return nil
	}
	ref := "scratchpad/" + slug + "/"
	var children []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == slug || e.Name() == "archive" {
			continue
		}
		if dirReferences(filepath.Join(scratchpad, e.Name()), ref) {
			children = append(children, e.Name())
		}
	}
	return children
}

func dirReferences(dir, ref string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found || d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), ref) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
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
	path := "Light close"
	if full {
		path = "Full close"
	}
	return fmt.Sprintf("%s for %s: %s.", path, f.slug, strings.Join(reasons, ", "))
}

func unfinishedNotice(f closeFacts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is not ready to close. These Plans items are unchecked:\n", f.slug)
	for _, item := range f.open {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	fmt.Fprintf(&b, "\nFinish each one, or mark it `[x]` if the code already shows it done, then run `lgrass how task close %s` again. If one cannot be finished, tell the user the task cannot close and why.\n", f.slug)
	return b.String()
}
