// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// heredocSpan is the byte range of one heredoc body in a command, closing delimiter excluded.
type heredocSpan struct {
	start, end int
	// dataOnly marks a body that only reaches a file: its command is cat or tee with stdout redirected to a file, so no command runs it.
	dataOnly bool
}

// shellHeredocs returns the heredoc bodies of command in order, and false when the command does not parse.
func shellHeredocs(command string) ([]heredocSpan, bool) {
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, false
	}
	var spans []heredocSpan
	syntax.Walk(file, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		for _, r := range stmt.Redirs {
			if (r.Op != syntax.Hdoc && r.Op != syntax.DashHdoc) || r.Hdoc == nil {
				continue
			}
			start, end := int(r.Hdoc.Pos().Offset()), int(r.Hdoc.End().Offset())
			end = start + strings.LastIndexByte(command[start:end], '\n') + 1
			spans = append(spans, heredocSpan{start: start, end: end, dataOnly: bodyOnlyReachesFile(stmt)})
		}
		return true
	})
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans, true
}

func bodyOnlyReachesFile(stmt *syntax.Stmt) bool {
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	name := call.Args[0].Lit()
	if name != "cat" && name != "tee" {
		return false
	}
	for _, r := range stmt.Redirs {
		if stdoutToFile(r) {
			return true
		}
	}
	return false
}

func stdoutToFile(r *syntax.Redirect) bool {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll:
	default:
		return false
	}
	if r.N != nil && r.N.Value != "1" {
		return false
	}
	target := r.Word.Lit()
	return !(strings.HasPrefix(target, "/dev/") && target != "/dev/null") && !strings.HasPrefix(target, "/proc/")
}

// firstMemoryMention is the earliest offset at which command names a memory directory, or -1.
func firstMemoryMention(command, projectMemDir string) int {
	first := -1
	if loc := claudeProjectsInCommand.FindStringIndex(command); loc != nil && strings.Contains(command, "memory") {
		first = loc[0]
	}
	if projectMemDir != "" {
		if i := strings.Index(command, strings.TrimSuffix(projectMemDir, "/")); i >= 0 && (first < 0 || i < first) {
			first = i
		}
	}
	return first
}

// maskBodies replaces each body in spans that keep selects with spaces, so offsets into command stay valid.
func maskBodies(command string, spans []heredocSpan, keep func(heredocSpan) bool) string {
	b := []byte(command)
	for _, sp := range spans {
		if !keep(sp) {
			continue
		}
		for i := sp.start; i < sp.end && i < len(b); i++ {
			b[i] = ' '
		}
	}
	return string(b)
}
