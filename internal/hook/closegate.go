// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/faizalv/lemongrass/internal/session"
)

var (
	howTaskClose     = regexp.MustCompile(`\blgrass\s+how\s+task\s+close\s+([^\s;&|]+)`)
	shellSegmentStop = regexp.MustCompile(`&&|\|\||[;|\n]`)
)

// closeGateDeny records a task close mark when the call runs `lgrass how task close`, and denies moving a task directory into the scratchpad archive before its mark exists.
func closeGateDeny(store *session.Store, payload hookEvent) string {
	if payload.TabID == "" || payload.ToolName != "Bash" {
		return ""
	}
	var in shellToolInput
	if err := json.Unmarshal(payload.ToolInput, &in); err != nil || in.Command == "" {
		return ""
	}
	for _, m := range howTaskClose.FindAllStringSubmatch(in.Command, -1) {
		store.MarkReady(payload.TabID, session.TaskCloseMark(taskSlug(m[1])))
	}
	slugs := archivedTaskSlugs(in.Command)
	if len(slugs) == 0 {
		return ""
	}
	marks, err := store.Marks(payload.TabID)
	if err != nil {
		return ""
	}
	for _, slug := range slugs {
		if !marks[session.TaskCloseMark(slug)] {
			return session.FormatTaskCloseDeny(slug)
		}
	}
	return ""
}

// archivedTaskSlugs returns the task directories an mv in command moves into the scratchpad archive.
func archivedTaskSlugs(command string) []string {
	var slugs []string
	for _, segment := range shellSegmentStop.Split(command, -1) {
		fields := strings.Fields(segment)
		if len(fields) > 1 && fields[0] == "git" {
			fields = fields[1:]
		}
		if len(fields) < 3 || fields[0] != "mv" {
			continue
		}
		var paths []string
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "-") {
				paths = append(paths, strings.Trim(f, `"'`))
			}
		}
		if len(paths) < 2 || !isScratchpadArchive(paths[len(paths)-1]) {
			continue
		}
		for _, src := range paths[:len(paths)-1] {
			if strings.Contains(src, "scratchpad/") && !strings.Contains(filepath.Clean(src)+"/", "scratchpad/archive/") {
				slugs = append(slugs, taskSlug(src))
			}
		}
	}
	return slugs
}

func isScratchpadArchive(p string) bool {
	return strings.HasSuffix(filepath.Clean(p), "scratchpad/archive")
}

func taskSlug(p string) string {
	return filepath.Base(strings.TrimRight(strings.Trim(p, `"'`), "/"))
}
