// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// subprocessCalls are the calls in each language that run a command, as a shell string or as a program and its arguments.
var subprocessCalls = map[string][]destructiveCall{
	"python": {
		{name: "os.system"}, {name: "os.popen"}, {name: "subprocess.run"}, {name: "subprocess.call"}, {name: "subprocess.check_call"},
		{name: "subprocess.check_output"}, {name: "subprocess.Popen"}, {name: "subprocess.getoutput"}, {name: "subprocess.getstatusoutput"},
	},
	"node": {
		{name: "exec", anyObject: true}, {name: "execSync", anyObject: true}, {name: "spawn", anyObject: true}, {name: "spawnSync", anyObject: true},
		{name: "execFile", anyObject: true}, {name: "execFileSync", anyObject: true}, {name: "execa", anyObject: true}, {name: "execaSync", anyObject: true},
	},
	"deno": {{name: "Deno.Command"}, {name: "Deno.run"}},
	"php":  {{name: "shell_exec"}, {name: "exec"}, {name: "system"}, {name: "passthru"}, {name: "proc_open"}, {name: "popen"}},
	"perl": {{name: "system"}, {name: "exec"}},
	"ruby": {
		{name: "system"}, {name: "exec"}, {name: "spawn"}, {name: "Process.spawn"}, {name: "IO.popen"},
		{name: "Open3.capture2"}, {name: "Open3.capture2e"}, {name: "Open3.capture3"}, {name: "Open3.popen3"},
	},
}

// backtickCommands marks languages where backticks run a shell command, unlike JavaScript template literals.
var backtickCommands = map[string]bool{"php": true, "perl": true, "ruby": true}

// subprocessCommands returns the shell commands code runs whose text is written out in full: a string literal, or a program and arguments that are all literals. A command built from variables is left out; the path and call passes still read it.
func subprocessCommands(lang, code string) []string {
	var commands []string
	for _, call := range subprocessCalls[lang] {
		for _, at := range callSites(lang, code, call) {
			if command, ok := subprocessCommandAt(lang, code, at); ok {
				commands = append(commands, command)
			}
		}
	}
	if backtickCommands[lang] {
		commands = append(commands, quotedCommands(lang, code)...)
	}
	return commands
}

func subprocessCommandAt(lang, code string, at int) (string, bool) {
	i := skipSpaces(code, at)
	paren := i < len(code) && code[i] == '('
	if paren {
		i++
	} else if !parenOptional[lang] {
		return "", false
	}
	i = skipSpacesAndNewlines(code, i)
	if i < len(code) && code[i] == '[' {
		words, end, ok := literalList(lang, code, i)
		return shellJoin(words), ok && len(words) > 0 && literalEnds(code, end, paren)
	}
	var words []string
	for {
		word, end, ok := parseLiteral(lang, code, i)
		if !ok || !literalEnds(code, end, paren) {
			if len(words) == 0 {
				return "", false
			}
			break
		}
		words = append(words, word)
		next := skipSpacesAndNewlines(code, end)
		if next >= len(code) || code[next] != ',' || !(lang == "perl" || lang == "ruby" || len(words) == 1) {
			break
		}
		next = skipSpacesAndNewlines(code, next+1)
		if next < len(code) && code[next] == '[' {
			list, listEnd, listOK := literalList(lang, code, next)
			if !listOK || !literalEnds(code, listEnd, paren) {
				return "", false
			}
			return shellJoin(append(words, list...)), true
		}
		if next < len(code) && code[next] == '{' {
			if list, ok := denoArgs(lang, code, next); ok {
				return shellJoin(append(words, list...)), true
			}
			break
		}
		if !(lang == "perl" || lang == "ruby") {
			break
		}
		i = next
	}
	if len(words) == 1 {
		return words[0], true
	}
	return shellJoin(words), true
}

// literalEnds reports a literal argument that is not part of a longer expression such as a concatenation.
func literalEnds(code string, end int, paren bool) bool {
	next := skipSpacesAndNewlines(code, end)
	return next >= len(code) || code[next] == ',' || argumentsEnd(code, next, paren)
}

// literalList reads a list whose items are all string literals, such as ["rm", "-rf", "x"].
func literalList(lang, code string, i int) ([]string, int, bool) {
	var words []string
	i++
	for {
		i = skipSpacesAndNewlines(code, i)
		if i < len(code) && code[i] == ']' {
			return words, i + 1, true
		}
		word, end, ok := parseLiteral(lang, code, i)
		if !ok {
			return nil, end, false
		}
		words = append(words, word)
		i = skipSpacesAndNewlines(code, end)
		if i < len(code) && code[i] == ',' {
			i++
		}
	}
}

// denoArgs reads the args list of a Deno.Command options object.
func denoArgs(lang, code string, i int) ([]string, bool) {
	if lang != "deno" {
		return nil, false
	}
	end := strings.IndexByte(code[i:], '}')
	if end < 0 {
		return nil, false
	}
	options := code[i : i+end]
	at := strings.Index(options, "args")
	if at < 0 {
		return nil, true
	}
	j := skipSpaces(options, at+len("args"))
	if j >= len(options) || options[j] != ':' {
		return nil, false
	}
	j = skipSpaces(options, j+1)
	if j >= len(options) || options[j] != '[' {
		return nil, false
	}
	list, _, ok := literalList(lang, code, i+j)
	return list, ok
}

// quotedCommands returns the commands in backticks and in qx or %x quoting whose text does not interpolate.
func quotedCommands(lang, code string) []string {
	var commands []string
	for i := 0; i < len(code); i++ {
		open := code[i]
		start := -1
		closer := byte(0)
		switch {
		case open == '`':
			start, closer = i+1, '`'
		case (lang == "perl" && strings.HasPrefix(code[i:], "qx")) || (lang == "ruby" && strings.HasPrefix(code[i:], "%x")):
			if (i > 0 && isIdentChar("", code[i-1])) || i+2 >= len(code) {
				continue
			}
			delim := code[i+2]
			if isIdentChar("", delim) || delim == ' ' {
				continue
			}
			start, closer = i+3, closingDelimiter(delim)
		default:
			continue
		}
		end := strings.IndexByte(code[start:], closer)
		if end < 0 {
			return commands
		}
		text := code[start : start+end]
		if !interpolates(lang, '"', text) {
			commands = append(commands, text)
		}
		i = start + end
	}
	return commands
}

func closingDelimiter(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '{':
		return '}'
	case '[':
		return ']'
	case '<':
		return '>'
	}
	return open
}

func skipSpacesAndNewlines(code string, i int) int {
	for i < len(code) && strings.IndexByte(" \t\r\n", code[i]) >= 0 {
		i++
	}
	return i
}

// shellJoin quotes each word so parsing the joined command gives back exactly these arguments.
func shellJoin(words []string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
