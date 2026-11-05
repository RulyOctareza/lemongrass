// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const maxShellDepth = 6

type shellCall struct {
	Name    string
	Path    string
	Dynamic bool
	Args    []string
	// ArgDynamic marks each argument holding a parameter expansion or substitution, parallel to Args; nil when the arguments came from text the parser did not see as words.
	ArgDynamic []bool
	// Indirect marks a call whose arguments or working directory come from xargs, find -exec or env --chdir at run time.
	Indirect bool
}

type shellRedirect struct {
	Path  string
	Write bool
}

type shellScript struct {
	Calls     []shellCall
	Redirects []shellRedirect
	Pipelines [][]string
	Programs  []shellProgram
	// Depth counts the script files this script was read through, bounding a script that runs itself.
	Depth int
}

// shellProgram is code the command hands to an interpreter: inline, from stdin, or a script file read when the command is judged.
type shellProgram struct {
	// Lang is python, node, php, perl, ruby, deno or sh, or "" for a path-named command whose shebang decides.
	Lang string
	Code string
	File string
	// At is how many calls precede the program, so the literal cd calls before it give its working directory.
	At int
}

type shellWrapper struct {
	valueFlags     map[string]bool
	skipPositional int
	skipAssigns    bool
}

func flagSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

var shellWrappers = map[string]shellWrapper{
	"env":     {valueFlags: flagSet("-u", "-C", "-S", "--unset", "--chdir", "--split-string"), skipAssigns: true},
	"nice":    {valueFlags: flagSet("-n", "--adjustment")},
	"nohup":   {},
	"setsid":  {},
	"time":    {valueFlags: flagSet("-f", "-o", "--format", "--output")},
	"command": {},
	"builtin": {},
	"exec":    {valueFlags: flagSet("-a")},
	"stdbuf":  {valueFlags: flagSet("-i", "-o", "-e", "--input", "--output", "--error")},
	"timeout": {valueFlags: flagSet("-s", "-k", "--signal", "--kill-after"), skipPositional: 1},
	"xargs":   {valueFlags: flagSet("-I", "-n", "-P", "-L", "-s", "-d", "-E", "-a", "--max-args", "--max-procs", "--max-lines", "--delimiter", "--arg-file", "--replace")},
	"ionice":  {valueFlags: flagSet("-c", "-n", "-p", "--class", "--classdata")},
	"watch":   {valueFlags: flagSet("-n", "--interval")},
}

var shellInterpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true}

func parseShell(command string) (shellScript, error) {
	var script shellScript
	if err := script.add(command, 0); err != nil {
		return shellScript{}, err
	}
	return script, nil
}

func (s *shellScript) add(command string, depth int) error {
	if depth > maxShellDepth {
		return errors.New("shell command nests too deeply to inspect")
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return err
	}
	var walkErr error
	syntax.Walk(file, func(node syntax.Node) bool {
		if walkErr != nil {
			return false
		}
		switch n := node.(type) {
		case *syntax.CallExpr:
			walkErr = s.addCallExpr(n, depth)
		case *syntax.BinaryCmd:
			if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
				s.Pipelines = append(s.Pipelines, pipelineNames(n))
				walkErr = s.addPipe(n, depth)
			}
		case *syntax.Stmt:
			walkErr = s.addRedirects(n, depth)
		}
		return walkErr == nil
	})
	return walkErr
}

func (s *shellScript) addCallExpr(call *syntax.CallExpr, depth int) error {
	if len(call.Args) == 0 {
		return nil
	}
	name, dynamic := wordText(call.Args[0])
	args := make([]string, 0, len(call.Args)-1)
	argDynamic := make([]bool, 0, len(call.Args)-1)
	for _, word := range call.Args[1:] {
		text, wordDynamic := wordText(word)
		args = append(args, text)
		argDynamic = append(argDynamic, wordDynamic)
	}
	return s.addCall(name, dynamic, args, argDynamic, false, depth)
}

func (s *shellScript) addCall(name string, dynamic bool, args []string, argDynamic []bool, indirect bool, depth int) error {
	base := commandBase(name)
	s.Calls = append(s.Calls, shellCall{Name: base, Path: name, Dynamic: dynamic, Args: args, ArgDynamic: argDynamic, Indirect: indirect})
	if dynamic {
		return nil
	}
	if wrapper, ok := shellWrappers[base]; ok {
		if inner := unwrapArgs(wrapper, args); len(inner) > 0 {
			innerIndirect := indirect || base == "xargs" || (base == "env" && changesDirectory(args))
			offset := len(args) - len(inner)
			return s.addCall(inner[0], false, inner[1:], sliceFlags(argDynamic, offset+1, len(args)), innerIndirect, depth+1)
		}
		return nil
	}
	if shellInterpreters[base] {
		if script, ok := commandStringArg(args); ok {
			return s.add(script, depth+1)
		}
		if file := shellScriptFile(args); file != "" {
			s.addProgram("sh", "", file, depth)
		}
		return nil
	}
	if lang := interpreterLanguage(base); lang != "" {
		run := parseInterpreterArgs(lang, args)
		s.addProgram(lang, run.Code, run.File, depth)
		return nil
	}
	switch base {
	case "eval":
		return s.add(strings.Join(args, " "), depth+1)
	case "find":
		return s.addFindExec(args, depth)
	case "source", ".":
		if len(args) > 0 {
			s.addProgram("sh", "", args[0], depth)
		}
		return nil
	}
	if namesPath(name) {
		s.addProgram("", "", name, depth)
	}
	return nil
}

func (s *shellScript) addProgram(lang, code, file string, depth int) {
	if code == "" && file == "" {
		return
	}
	s.Programs = append(s.Programs, shellProgram{Lang: lang, Code: code, File: file, At: len(s.Calls)})
	if code == "" || lang == "sh" {
		return
	}
	for _, command := range subprocessCommands(lang, code) {
		// A command found by text matching may not be valid shell; it is judged when it parses and skipped when it does not.
		_ = s.add(command, depth+1)
	}
}

func (s *shellScript) addFindExec(args []string, depth int) error {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
			end := i + 1
			for end < len(args) && args[end] != ";" && args[end] != "+" {
				end++
			}
			if end > i+1 {
				if err := s.addCall(args[i+1], false, args[i+2:end], nil, true, depth+1); err != nil {
					return err
				}
			}
			i = end
		}
	}
	return nil
}

func (s *shellScript) addRedirects(stmt *syntax.Stmt, depth int) error {
	base, args, _ := stmtCommand(stmt)
	interpreter := shellInterpreters[base]
	lang := stdinLanguage(base, args)
	for _, redirect := range stmt.Redirs {
		switch redirect.Op {
		case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			text, ok := heredocText(redirect)
			switch {
			case !ok:
			case interpreter:
				if err := s.add(text, depth+1); err != nil {
					return err
				}
			case lang != "":
				s.addProgram(lang, text, "", depth)
			}
		default:
			if redirect.Word == nil {
				continue
			}
			text, _ := wordText(redirect.Word)
			s.Redirects = append(s.Redirects, shellRedirect{Path: text, Write: redirectWrites(redirect.Op)})
			if redirect.Op == syntax.RdrIn && lang != "" {
				s.addProgram(lang, "", text, depth)
			}
		}
	}
	return nil
}

func heredocText(redirect *syntax.Redirect) (string, bool) {
	word := redirect.Hdoc
	if redirect.Op == syntax.WordHdoc {
		word = redirect.Word
	}
	if word == nil {
		return "", false
	}
	text, _ := wordText(word)
	return text, true
}

func redirectWrites(op syntax.RedirOperator) bool {
	switch op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrInOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll, syntax.AppClob, syntax.DplOut:
		return true
	}
	return false
}

func pipelineNames(cmd *syntax.BinaryCmd) []string {
	var names []string
	for _, stmt := range []*syntax.Stmt{cmd.X, cmd.Y} {
		switch inner := stmt.Cmd.(type) {
		case *syntax.BinaryCmd:
			if inner.Op == syntax.Pipe || inner.Op == syntax.PipeAll {
				names = append(names, pipelineNames(inner)...)
				continue
			}
			names = append(names, "")
		case *syntax.CallExpr:
			if len(inner.Args) == 0 {
				names = append(names, "")
				continue
			}
			name, dynamic := wordText(inner.Args[0])
			if dynamic {
				name = ""
			}
			names = append(names, commandBase(name))
		default:
			names = append(names, "")
		}
	}
	return names
}

func changesDirectory(args []string) bool {
	for _, arg := range args {
		if arg == "-C" || arg == "--chdir" || strings.HasPrefix(arg, "--chdir=") || (strings.HasPrefix(arg, "-C") && len(arg) > 2) {
			return true
		}
	}
	return false
}

func unwrapArgs(wrapper shellWrapper, args []string) []string {
	skip := wrapper.skipPositional
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return trimSkipped(args[i+1:], skip)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			if wrapper.valueFlags[arg] && !strings.Contains(arg, "=") {
				i++
			}
		case wrapper.skipAssigns && strings.Contains(arg, "="):
		case skip > 0:
			skip--
		default:
			return args[i:]
		}
	}
	return nil
}

func trimSkipped(args []string, skip int) []string {
	if skip >= len(args) {
		return nil
	}
	return args[skip:]
}

func commandStringArg(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
			return "", false
		}
		if strings.Contains(arg, "c") {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
	}
	return "", false
}

func dynamicAt(flags []bool, i int) bool {
	return i >= 0 && i < len(flags) && flags[i]
}

func sliceFlags(flags []bool, from, to int) []bool {
	if flags == nil || from > to || to > len(flags) {
		return nil
	}
	return flags[from:to]
}

func wordText(word *syntax.Word) (string, bool) {
	var b strings.Builder
	dynamic := writeWordParts(&b, word.Parts, false)
	return b.String(), dynamic
}

func writeWordParts(b *strings.Builder, parts []syntax.WordPart, quoted bool) bool {
	dynamic := false
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if quoted {
				b.WriteString(unescapeDoubleQuoted(p.Value))
			} else {
				b.WriteString(unescapeLiteral(p.Value))
			}
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if writeWordParts(b, p.Parts, true) {
				dynamic = true
			}
		case *syntax.ParamExp:
			b.WriteString("$" + p.Param.Value)
			dynamic = true
		default:
			b.WriteString("$(dynamic)")
			dynamic = true
		}
	}
	return dynamic
}

func unescapeLiteral(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			if value[i] == '\n' {
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}
