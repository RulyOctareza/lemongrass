// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"strings"
)

// Catalog descriptions of the interpreter rules, shared with the catalog entries so the verdict and the Safety pane say the same thing.
const (
	DynamicTargetDescription     = "deleting or changing permissions on a path built from a variable or substitution"
	InterpreterDeleteDescription = "interpreter code deleting or moving a path that is not a literal inside the project"
	ScriptDeleteDescription      = "a script run by an interpreter deletes or moves a path that is not a literal inside the project"
	ScriptCommandDescription     = "a command in a script file run by an interpreter that a catalog rule guards"
)

var languageNames = map[string]string{"python": "Python", "node": "Node", "deno": "Deno", "php": "PHP", "perl": "Perl", "ruby": "Ruby"}

// interpreterVerdicts judges the code a command hands to interpreters, inline or as script files. It is a tripwire for code that says what it means in plain text, not a defence against code built to hide.
func (c checker) interpreterVerdicts(script shellScript) []Verdict {
	var found []Verdict
	for _, program := range script.Programs {
		dir, known := c.programDir(script, program.At)
		if program.Code != "" && program.Lang != "sh" {
			where := languageNames[program.Lang] + " code"
			found = append(found, c.codeVerdicts(program.Lang, program.Code, dir, known, "interpreter-delete", where)...)
		}
		if program.File != "" {
			found = append(found, c.scriptFileVerdicts(script, program, dir, known)...)
		}
	}
	return found
}

// programDir replays the literal cd and pushd calls before a program to find the directory it runs in; known is false once a cd cannot be followed.
func (c checker) programDir(script shellScript, at int) (string, bool) {
	dir := c.in.Cwd
	known := dir != ""
	for i := 0; i < at && i < len(script.Calls); i++ {
		call := script.Calls[i]
		if !directoryChangers[call.Name] {
			continue
		}
		target, ok := directoryTarget(call)
		if !ok || !literalTarget(target) || !known {
			known = false
			continue
		}
		dir = ResolvePath(target, dir, c.home)
	}
	return dir, known
}

// codeVerdicts runs the path pass and the call pass over one program's text.
func (c checker) codeVerdicts(lang, code, dir string, known bool, ruleID, where string) []Verdict {
	var found []Verdict
	for _, token := range pathTokens(code) {
		found = append(found, c.pathChecks(token, true)...)
	}
	if detail := c.destructiveCallDetail(lang, code, dir, known); detail != "" {
		found = append(found, ruleVerdict(ruleID, "In "+where+", "+detail))
	}
	return found
}

// pathTokens splits code into the words that could name a path: anything with a slash, or starting with a dot or a tilde, such as '.ssh' or "~/.aws/credentials".
func pathTokens(code string) []string {
	seen := map[string]bool{}
	var tokens []string
	for _, token := range strings.FieldsFunc(code, isTokenSeparator) {
		if seen[token] || !(strings.Contains(token, "/") || token[0] == '.' || token[0] == '~') {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

func isTokenSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\'', '"', '`', '(', ')', '[', ']', '{', '}', ',', '+', ';', '=', '<', '>':
		return true
	}
	return false
}

// destructiveCallDetail returns why the first destructive call in code does not only touch literal paths inside the project, or "" when none is found.
func (c checker) destructiveCallDetail(lang, code, dir string, known bool) string {
	for _, call := range destructiveCalls[lang] {
		for _, at := range callSites(lang, code, call) {
			if detail := c.callSiteDetail(lang, code, call, at, dir, known); detail != "" {
				return detail
			}
		}
	}
	if namedByString[lang] {
		for _, call := range destructiveCalls[lang] {
			for _, quote := range []string{"'", `"`} {
				if strings.Contains(code, quote+call.name+quote) {
					return call.name + " is passed by name, so the paths it gets are unknown until it runs. Call it on each path written out as a string inside the project."
				}
			}
		}
	}
	return ""
}

// callSites returns the offsets just past each whole-word occurrence of a call's name.
func callSites(lang, code string, call destructiveCall) []int {
	var sites []int
	for from := 0; ; {
		i := strings.Index(code[from:], call.name)
		if i < 0 {
			return sites
		}
		start := from + i
		end := start + len(call.name)
		from = end
		if end < len(code) && isIdentChar(lang, code[end]) {
			continue
		}
		if start > 0 {
			prev := code[start-1]
			switch {
			case call.receiver:
				if prev != '.' {
					continue
				}
			case prev == '.':
				if !call.anyObject {
					continue
				}
			case isIdentChar(lang, prev):
				continue
			}
		} else if call.receiver {
			continue
		}
		sites = append(sites, end)
	}
}

func isIdentChar(lang string, b byte) bool {
	if b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}
	if lang == "perl" || lang == "ruby" || lang == "php" {
		return b == '@' || b == '%' || b == '&' || b == ':'
	}
	return false
}

func (c checker) callSiteDetail(lang, code string, call destructiveCall, at int, dir string, known bool) string {
	name := call.name
	if call.receiver {
		name = "." + name
		receiver, ok := pathReceiver(lang, code, at-len(call.name)-1)
		if !ok {
			return name + "() is called on a path held in a variable or expression, so where it lands is unknown until it runs. Use Path(\"...\") with the path written out inside the project."
		}
		if reason := c.literalPathReason(receiver, dir, known); reason != "" {
			return name + "() on " + receiver + ": " + reason
		}
	}
	i := skipSpaces(code, at)
	paren := i < len(code) && code[i] == '('
	if paren {
		i++
	} else if !parenOptional[lang] {
		if passedAsValue(code, at-len(call.name), i) {
			return name + " is passed as a function, so the paths it gets are unknown until it runs. Call it on each path written out as a string inside the project."
		}
		return ""
	}
	for k := 0; call.paths < 0 || k < call.paths; k++ {
		i = skipArgumentSpace(code, i, paren)
		if argumentsEnd(code, i, paren) {
			if k == 0 && lang == "perl" {
				return name + " with no argument acts on $_, which is unknown until it runs. Write the path out as a string inside the project."
			}
			return ""
		}
		value, end, ok := parseLiteral(lang, code, i)
		if ok {
			next := skipArgumentSpace(code, end, paren)
			ok = next >= len(code) || code[next] == ',' || argumentsEnd(code, next, paren)
			end = next
		}
		if !ok {
			return name + "(" + argumentText(code, i) + ") takes a variable or expression, so where it lands is unknown until it runs. Write the path out as a string literal inside the project."
		}
		if reason := c.literalPathReason(value, dir, known); reason != "" {
			return name + "(\"" + value + "\"): " + reason
		}
		if end >= len(code) || code[end] != ',' {
			return ""
		}
		i = end + 1
	}
	return ""
}

func skipSpaces(code string, i int) int {
	for i < len(code) && (code[i] == ' ' || code[i] == '\t') {
		i++
	}
	return i
}

// skipArgumentSpace skips the space between arguments, including line breaks inside parentheses, where a line break does not end the call.
func skipArgumentSpace(code string, i int, paren bool) int {
	if paren {
		return skipSpacesAndNewlines(code, i)
	}
	return skipSpaces(code, i)
}

func argumentsEnd(code string, i int, paren bool) bool {
	if i >= len(code) {
		return true
	}
	if paren {
		return code[i] == ')'
	}
	return code[i] == ';' || code[i] == '\n' || code[i] == ')' || code[i] == '}'
}

// passedAsValue reports a function name handed to another call as an argument, as in map(os.remove, files) or forEach(fs.unlinkSync), but not a name listed in an import.
func passedAsValue(code string, start, after int) bool {
	prev := start - 1
	for prev >= 0 && code[prev] == '.' || prev >= 0 && isIdentChar("", code[prev]) {
		prev--
	}
	for prev >= 0 && (code[prev] == ' ' || code[prev] == '\t') {
		prev--
	}
	if prev < 0 || after >= len(code) {
		return false
	}
	before, next := code[prev], code[after]
	return (before == '(' && (next == ',' || next == ')')) || (before == ',' && next == ')')
}

func argumentText(code string, i int) string {
	end := i
	depth := 0
	for end < len(code) && end-i < 60 {
		switch code[end] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return code[i:end]
			}
			depth--
		case ',', ';', '\n':
			if depth == 0 {
				return code[i:end]
			}
		}
		end++
	}
	return code[i:end]
}

// parseLiteral reads a plain string literal at i, refusing any form that interpolates.
func parseLiteral(lang, code string, i int) (string, int, bool) {
	if lang == "python" {
		for i < len(code) && strings.IndexByte("rRbBuU", code[i]) >= 0 {
			i++
		}
	}
	if i >= len(code) {
		return "", i, false
	}
	quote := code[i]
	switch {
	case quote == '\'' || quote == '"':
	case quote == '`' && (lang == "node" || lang == "deno"):
	default:
		return "", i, false
	}
	var b strings.Builder
	for j := i + 1; j < len(code); j++ {
		ch := code[j]
		if ch == '\\' && j+1 < len(code) {
			j++
			b.WriteByte(code[j])
			continue
		}
		if ch == quote {
			value := b.String()
			if interpolates(lang, quote, value) {
				return "", j + 1, false
			}
			return value, j + 1, true
		}
		b.WriteByte(ch)
	}
	return "", len(code), false
}

func interpolates(lang string, quote byte, value string) bool {
	switch {
	case quote == '`':
		return strings.Contains(value, "${")
	case quote == '"' && (lang == "php" || lang == "perl"):
		return strings.ContainsAny(value, "$@")
	case quote == '"' && lang == "ruby":
		return strings.Contains(value, "#{")
	}
	return false
}

// pathReceiver returns the literal inside Path("...") ending just before a method's dot.
func pathReceiver(lang, code string, dot int) (string, bool) {
	end := dot - 1
	if end < 0 || code[end] != ')' {
		return "", false
	}
	depth := 0
	open := -1
	for i := end; i >= 0; i-- {
		if code[i] == ')' {
			depth++
		} else if code[i] == '(' {
			depth--
			if depth == 0 {
				open = i
				break
			}
		}
	}
	if open < 0 {
		return "", false
	}
	nameEnd := open
	nameStart := nameEnd
	for nameStart > 0 && (isIdentChar(lang, code[nameStart-1]) || code[nameStart-1] == '.') {
		nameStart--
	}
	if !strings.HasSuffix(code[nameStart:nameEnd], "Path") {
		return "", false
	}
	value, after, ok := parseLiteral(lang, code, skipSpaces(code, open+1))
	if !ok || skipSpaces(code, after) != end {
		return "", false
	}
	return value, true
}

// literalPathReason returns why a literal path a destructive call acts on is not plainly inside the project, or "" when it is.
func (c checker) literalPathReason(value, dir string, known bool) string {
	root := c.projectRoot()
	if root == "" {
		return "this session has no launch project, so every deletion needs the user."
	}
	if !known && !writtenAbs(value) {
		return "the working directory is unknown after a cd the guard cannot follow, so the relative path cannot be checked. Write it from the project root."
	}
	p := ResolvePath(value, dir, c.home)
	location := locate(p)
	for _, candidate := range []string{location, resolveFully(p)} {
		if reason := c.candidateCheck(value, candidate, dir, []string{dir}, false); reason != "" {
			return reason
		}
	}
	return ""
}
