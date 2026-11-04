// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The reserved files that say what kind of work a task directory holds.
var taskKinds = []string{"prd.md", "recon.md", "debug.md", "notes.md"}

type workFacts struct {
	slug     string
	exists   bool
	archived bool
	files    []string
	handover string
}

func (f workFacts) has(name string) bool {
	for _, file := range f.files {
		if file == name {
			return true
		}
	}
	return false
}

func inspectWork(root, slug string) workFacts {
	scratchpad := filepath.Join(root, "biblio", "scratchpad")
	f := workFacts{slug: slug, handover: findHandover(root, slug)}
	if info, err := os.Stat(filepath.Join(scratchpad, slug)); err == nil && info.IsDir() {
		f.exists = true
		for _, name := range taskKinds {
			if _, err := os.Stat(filepath.Join(scratchpad, slug, name)); err == nil {
				f.files = append(f.files, name)
			}
		}
	} else if _, err := os.Stat(filepath.Join(scratchpad, "archive", slug)); err == nil {
		f.archived = true
	}
	return f
}

// archivedNotice stops work under a slug whose task is already archived, since a new directory would shadow its record.
func archivedNotice(slug string) string {
	return fmt.Sprintf("%s is an archived task in biblio/scratchpad/archive/%s. Pick another slug for new work, or ask the user whether the archived task should be reopened.\n", slug, slug)
}

func handoverLead(f workFacts) string {
	if f.handover == "" {
		return ""
	}
	return "Read the Status of `biblio/handover/" + f.handover + "` first: it says where to resume and what is blocked. "
}

func workTask(root string, args []string) (string, error) {
	slug, err := parseSlug(args, "lgrass how work task <slug>")
	if err != nil {
		return "", err
	}
	f := inspectWork(root, slug)
	if f.archived {
		return archivedNotice(slug), nil
	}
	// books and prdcheck are fragments of their own so a resumed or research-made PRD gets the same books step and stale check as a new task.
	vars := map[string]string{"slug": slug, "handover": handoverLead(f), "existing": "", "books": fragment("work/books.md", nil), "prdcheck": fragment("work/prdcheck.md", nil)}

	var b strings.Builder
	switch {
	case f.has("prd.md"):
		plans, _, open := readPRD(filepath.Join(root, "biblio", "scratchpad", slug, "prd.md"))
		if plans == 0 {
			fmt.Fprintf(&b, "Task %s: prd.md has no Plans items.\n\n## Steps\n\n", slug)
			b.WriteString(fragment("work/noplans.md", vars) + "\n\n" + fragment("work/plan.md", vars))
			break
		}
		if len(open) == 0 {
			fmt.Fprintf(&b, "Task %s: all %d Plans items are marked done. If the work is finished, close it with `lgrass how task close %s`. If more is needed, add it to Plans as `- [ ]` and tell the user, since that changes what the task promised.\n", slug, plans, slug)
			return b.String(), nil
		}
		vars["first"] = "\"" + strings.TrimRight(open[0], ".") + "\""
		fmt.Fprintf(&b, "Resume task %s: %d of %d Plans items open.\n\n## Steps\n\n", slug, len(open), plans)
		b.WriteString(fragment("work/resume.md", vars))
	default:
		if f.exists {
			fmt.Fprintf(&b, "Task %s exists without a prd.md (%s).\n\n", slug, filesOrNone(f.files))
			vars["existing"] = " Build on what the files already in the directory settled, and keep them beside the PRD."
		} else {
			fmt.Fprintf(&b, "New task %s.\n\n", slug)
		}
		b.WriteString("## Steps\n\n" + fragment("work/books.md", vars) + "\n\n" + fragment("work/plan.md", vars))
	}
	b.WriteString("\n\n" + fragment("work/shared.md", vars) + "\n")
	return b.String(), nil
}

// workDiscussion is a manual, but its books step is the shared fragment so the rule for reading books lives in one file.
func workDiscussion(_ string, args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("how: work/discussion takes no arguments")
	}
	return fragment("work/discussion.md", map[string]string{"books": fragment("work/books.md", nil)}) + "\n", nil
}

func workResearch(root string, args []string) (string, error) {
	return workRecord(root, args, "research", "recon.md",
		"Read `biblio/scratchpad/{slug}/recon.md` and `activities.md` and continue from what is still open.",
		"Create `biblio/scratchpad/{slug}/recon.md` with the question being investigated, and `activities.md` beside it. Then {books}")
}

func workDebug(root string, args []string) (string, error) {
	return workRecord(root, args, "debug", "debug.md",
		"Read `biblio/scratchpad/{slug}/debug.md` and `activities.md`, and continue from the current theory.",
		"Create `biblio/scratchpad/{slug}/debug.md` with the symptom as observed, exactly, and `activities.md` beside it. Then {books}")
}

// workRecord serves the kinds of work that keep one file rewritten in place, branching on whether that file exists yet.
func workRecord(root string, args []string, kind, file, resume, start string) (string, error) {
	slug, err := parseSlug(args, "lgrass how work "+kind+" <slug>")
	if err != nil {
		return "", err
	}
	f := inspectWork(root, slug)
	if f.archived {
		return archivedNotice(slug), nil
	}
	vars := map[string]string{"slug": slug}
	var b strings.Builder
	first := start
	if f.has(file) {
		first = handoverLead(f) + resume
		fmt.Fprintf(&b, "Continue %s %s.\n\n", kind, slug)
	} else {
		fmt.Fprintf(&b, "New %s %s.\n\n", kind, slug)
	}
	books := strings.TrimPrefix(fragment("work/books.md", nil), "**Books first.** Read")
	first = strings.ReplaceAll(first, "{books}", "read"+books)
	first = strings.ReplaceAll(first, "{slug}", slug)
	vars["recon"], vars["debug"] = first, first
	b.WriteString(fragment("work/"+kind+".md", vars) + "\n\n" + fragment("work/shared.md", vars) + "\n")
	return b.String(), nil
}

func filesOrNone(files []string) string {
	if len(files) == 0 {
		return "no reserved files"
	}
	return "has " + strings.Join(files, ", ")
}
