// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxScriptBytes caps a script file the guard reads. A hand-written script is far smaller; larger files are bundles and generated code, which pass unscanned.
const maxScriptBytes = 256 << 10

// maxScriptDepth bounds scripts that run other scripts, including one that runs itself.
const maxScriptDepth = 4

// scriptFileVerdicts reads a script file a command runs and judges its text: shell through the full catalog, other languages through the code scan. Every verdict except the locked lemongrass ones is re-labelled script-delete or script-command, so a project can switch script scanning off without touching the rule for the same command typed directly.
func (c checker) scriptFileVerdicts(script shellScript, program shellProgram, dir string, known bool) []Verdict {
	if script.Depth >= maxScriptDepth || (!known && !filepath.IsAbs(program.File)) {
		return nil
	}
	path := ResolvePath(program.File, dir, c.home)
	text, ok := readScript(path, program.Lang == "")
	if !ok {
		return nil
	}
	lang := program.Lang
	if lang == "" {
		if lang = shebangLanguage(text); lang == "" {
			return nil
		}
	}
	nested := c
	nested.in.Cwd = ""
	if known {
		nested.in.Cwd = dir
	}
	var found []Verdict
	if lang == "sh" {
		found = nested.nestedShellVerdicts(text, script.Depth+1)
	} else {
		found = nested.codeVerdicts(lang, text, dir, known, "script-delete", "script "+program.File)
		for _, command := range subprocessCommands(lang, text) {
			found = append(found, nested.nestedShellVerdicts(command, script.Depth+1)...)
		}
	}
	return c.scriptLabels(found, program.File)
}

// nestedShellVerdicts judges shell text read from a script file. A computed command name passes and text that does not parse is not judged: real scripts such as gradlew run "$JAVACMD", and the locked dynamic-command and unparseable-command verdicts would make them unrunnable with no switch.
func (c checker) nestedShellVerdicts(text string, depth int) []Verdict {
	nested, err := parseShell(text)
	if err != nil {
		return nil
	}
	nested.Depth = depth
	for i := range nested.Calls {
		nested.Calls[i].Dynamic = false
	}
	return c.scriptVerdicts(nested)
}

// scriptLabels applies each verdict's own policy first, so a rule the project allows stays allowed inside scripts, then re-labels what is left with a script rule id.
func (c checker) scriptLabels(found []Verdict, file string) []Verdict {
	var labelled []Verdict
	for _, verdict := range found {
		switch {
		case isLocked(verdict.RuleID), verdict.RuleID == "script-delete", verdict.RuleID == "script-command":
			labelled = append(labelled, verdict)
			continue
		}
		adjusted := c.pick([]Verdict{verdict})
		if adjusted == nil {
			continue
		}
		if verdict.RuleID == "interpreter-delete" {
			relabelled := ruleVerdict("script-delete", "In script "+file+": "+verdict.Detail)
			relabelled.Mode = adjusted.Mode
			labelled = append(labelled, relabelled)
			continue
		}
		detail := "In script " + file + ", rule " + verdict.RuleID + " (" + verdict.Description + ")"
		if verdict.Detail != "" {
			detail += ": " + verdict.Detail
		} else {
			detail += "."
		}
		relabelled := ruleVerdict("script-command", detail)
		relabelled.Mode = adjusted.Mode
		labelled = append(labelled, relabelled)
	}
	return labelled
}

// readScript reads a regular file under the cap; a FIFO or device is never opened, so judging a command can never block on it. With needShebang it reads past the first bytes only for a file that starts with #!.
func readScript(path string, needShebang bool) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScriptBytes {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	if needShebang {
		head := make([]byte, 2)
		if _, err := io.ReadFull(file, head); err != nil || string(head) != "#!" {
			return "", false
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return "", false
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxScriptBytes))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// shebangLanguage names the language a #! line runs, including through /usr/bin/env, or "" for an unknown interpreter.
func shebangLanguage(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	base := filepath.Base(fields[0])
	if base == "env" {
		base = ""
		for _, field := range fields[1:] {
			if !strings.HasPrefix(field, "-") && !strings.Contains(field, "=") {
				base = filepath.Base(field)
				break
			}
		}
	}
	if shellInterpreters[base] {
		return "sh"
	}
	return interpreterLanguage(base)
}
