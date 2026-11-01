// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// The hook files a command ask only for one plain command, the one the guard stopped. A call that chains commands or carries code would put a whole script in front of the human as one approval, so it is refused without an ask and the model is told to run the guarded command on its own.

// codeRunners run code given to them rather than one fixed program, so their call is never filed whole.
var codeRunners = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true, "eval": true, "source": true, ".": true,
	"python": true, "python2": true, "python3": true, "node": true, "nodejs": true, "perl": true, "ruby": true, "php": true,
	"deno": true, "bun": true, "lua": true, "osascript": true, "xargs": true,
}

// commandWrappers run the command named in their arguments, so a code runner behind one counts as the command itself.
var commandWrappers = map[string]bool{
	"env": true, "nice": true, "nohup": true, "timeout": true, "stdbuf": true, "time": true, "command": true,
	"exec": true, "setsid": true, "ionice": true, "sudo": true, "doas": true, "watch": true,
}

// simpleCommand reports whether command is one plain command and, when it is not, what it holds instead, worded to follow "this call holds".
func simpleCommand(command string) (string, bool) {
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return "shell text that cannot be parsed", false
	}
	if len(file.Stmts) != 1 {
		return "several commands", false
	}
	stmt := file.Stmts[0]
	if stmt.Background || stmt.Coprocess {
		return "a command sent to the background", false
	}
	for _, r := range stmt.Redirs {
		if r.Hdoc != nil || r.Op == syntax.Hdoc || r.Op == syntax.DashHdoc || r.Op == syntax.WordHdoc {
			return "a heredoc or herestring", false
		}
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return shapeOf(stmt.Cmd), false
	}
	if len(call.Args) == 0 {
		return "only variable assignments", false
	}
	if reason := substitutionIn(stmt); reason != "" {
		return reason, false
	}
	name, literal := literalWord(call.Args[0])
	if !literal {
		return "a command name computed at run time", false
	}
	base := runnerName(name)
	if codeRunners[base] {
		return "code run by " + base, false
	}
	if base == "find" && hasExec(call.Args[1:]) {
		return "commands run by find -exec", false
	}
	if commandWrappers[base] {
		for _, arg := range call.Args[1:] {
			if text, ok := literalWord(arg); ok && codeRunners[runnerName(text)] {
				return "code run by " + runnerName(text), false
			}
		}
	}
	return "", true
}

// runnerName folds versioned interpreter names such as python3.12 onto their family.
func runnerName(name string) string {
	base := filepath.Base(name)
	for _, family := range []string{"python3", "python2", "python", "perl", "ruby", "php", "node", "lua"} {
		if strings.HasPrefix(base, family) && strings.Trim(strings.TrimPrefix(base, family), "0123456789.") == "" {
			return family
		}
	}
	return base
}

func shapeOf(cmd syntax.Command) string {
	switch c := cmd.(type) {
	case *syntax.BinaryCmd:
		switch c.Op {
		case syntax.Pipe, syntax.PipeAll:
			return "a pipeline of several commands"
		}
		return "several commands chained with && or ||"
	case *syntax.Subshell, *syntax.Block:
		return "a subshell or block of commands"
	case *syntax.FuncDecl:
		return "a function definition"
	case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause:
		return "shell control flow"
	}
	return "more than one plain command"
}

func substitutionIn(stmt *syntax.Stmt) string {
	reason := ""
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if reason != "" {
			return false
		}
		switch node.(type) {
		case *syntax.CmdSubst:
			reason = "a command substitution"
		case *syntax.ProcSubst:
			reason = "a process substitution"
		}
		return reason == ""
	})
	return reason
}

func literalWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func hasExec(args []*syntax.Word) bool {
	for _, arg := range args {
		if text, ok := literalWord(arg); ok && (text == "-exec" || text == "-execdir" || text == "-ok" || text == "-okdir") {
			return true
		}
	}
	return false
}
