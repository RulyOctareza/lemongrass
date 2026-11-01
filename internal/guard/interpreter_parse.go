// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// stmtCommand returns the command a statement runs once wrappers such as env and timeout are peeled off, with literal argument text.
func stmtCommand(stmt *syntax.Stmt) (string, []string, bool) {
	if stmt == nil {
		return "", nil, false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", nil, false
	}
	name, dynamic := wordText(call.Args[0])
	if dynamic {
		return "", nil, false
	}
	args := make([]string, 0, len(call.Args)-1)
	for _, word := range call.Args[1:] {
		text, _ := wordText(word)
		args = append(args, text)
	}
	base := filepath.Base(name)
	for depth := 0; depth < maxShellDepth; depth++ {
		wrapper, ok := shellWrappers[base]
		if !ok {
			break
		}
		inner := unwrapArgs(wrapper, args)
		if len(inner) == 0 {
			return base, args, true
		}
		base, args = filepath.Base(inner[0]), inner[1:]
	}
	return base, args, true
}

// stdinLanguage names the language a command runs its stdin as, or "" when stdin is data or the command is no interpreter.
func stdinLanguage(base string, args []string) string {
	if shellInterpreters[base] {
		if _, ok := commandStringArg(args); ok || shellScriptFile(args) != "" {
			return ""
		}
		return "sh"
	}
	lang := interpreterLanguage(base)
	if lang == "" || !parseInterpreterArgs(lang, args).Stdin {
		return ""
	}
	return lang
}

var shellValueFlags = flagSet("-o", "+o", "-O", "+O", "--rcfile", "--init-file")

// shellScriptFile returns the script file a shell is given, or "" when it reads stdin or runs a -c string.
func shellScriptFile(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		case shellValueFlags[arg]:
			i++
		case strings.HasPrefix(arg, "--"):
		case (strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+")) && len(arg) > 1:
			if strings.ContainsAny(arg[1:], "cs") {
				return ""
			}
		default:
			return arg
		}
	}
	return ""
}

// addPipe treats text an echo, printf or cat stage feeds into an interpreter's stdin as that interpreter's code.
func (s *shellScript) addPipe(cmd *syntax.BinaryCmd, depth int) error {
	base, args, ok := stmtCommand(firstStage(cmd.Y))
	if !ok {
		return nil
	}
	lang := stdinLanguage(base, args)
	if lang == "" {
		return nil
	}
	text, files := pipeSource(lastStage(cmd.X))
	for _, file := range files {
		s.addProgram(lang, "", file, depth)
	}
	if text == "" {
		return nil
	}
	if lang == "sh" {
		return s.add(text, depth+1)
	}
	s.addProgram(lang, text, "", depth)
	return nil
}

func firstStage(stmt *syntax.Stmt) *syntax.Stmt {
	for stmt != nil {
		pipe, ok := stmt.Cmd.(*syntax.BinaryCmd)
		if !ok || (pipe.Op != syntax.Pipe && pipe.Op != syntax.PipeAll) {
			return stmt
		}
		stmt = pipe.X
	}
	return nil
}

func lastStage(stmt *syntax.Stmt) *syntax.Stmt {
	for stmt != nil {
		pipe, ok := stmt.Cmd.(*syntax.BinaryCmd)
		if !ok || (pipe.Op != syntax.Pipe && pipe.Op != syntax.PipeAll) {
			return stmt
		}
		stmt = pipe.Y
	}
	return nil
}

// pipeSource returns the text an echo, printf or heredoc cat stage writes, or the files a cat stage reads.
func pipeSource(stmt *syntax.Stmt) (string, []string) {
	base, args, ok := stmtCommand(stmt)
	if !ok {
		return "", nil
	}
	switch base {
	case "echo":
		escapes, i := false, 0
		for ; i < len(args) && isEchoFlag(args[i]); i++ {
			if strings.Contains(args[i], "e") {
				escapes = true
			}
		}
		text := strings.Join(args[i:], " ")
		if escapes {
			text = unescapePrintf(text)
		}
		return text, nil
	case "printf":
		if len(args) == 0 || args[0] == "-v" {
			return "", nil
		}
		if args[0] == "--" {
			args = args[1:]
		}
		return printfText(args), nil
	case "cat":
		var files []string
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				files = append(files, arg)
			}
		}
		if len(files) > 0 {
			return "", files
		}
		for _, redirect := range stmt.Redirs {
			switch redirect.Op {
			case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
				if text, ok := heredocText(redirect); ok {
					return text, nil
				}
			case syntax.RdrIn:
				if redirect.Word != nil {
					text, _ := wordText(redirect.Word)
					return "", []string{text}
				}
			}
		}
	}
	return "", nil
}

func isEchoFlag(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	return strings.Trim(arg[1:], "neE") == ""
}

// printfText approximates what printf writes: escapes in the format are expanded and each conversion takes the next argument.
func printfText(args []string) string {
	format, rest := args[0], args[1:]
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		if format[i] == '%' {
			b.WriteByte('%')
			continue
		}
		for i < len(format) && strings.IndexByte("-+ #0123456789.", format[i]) >= 0 {
			i++
		}
		if len(rest) > 0 {
			b.WriteString(rest[0])
			rest = rest[1:]
		}
	}
	text := unescapePrintf(b.String())
	if len(rest) > 0 {
		text += "\n" + strings.Join(rest, "\n")
	}
	return text
}

func unescapePrintf(text string) string {
	return strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\\`, `\`, `\'`, `'`, `\"`, `"`).Replace(text)
}
