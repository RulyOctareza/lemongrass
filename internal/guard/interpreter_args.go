// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"regexp"
	"strings"
)

var versionedInterpreter = regexp.MustCompile(`^(python|php|perl|ruby)[0-9.]*$`)

// interpreterLanguage names the language a command runs code in, or "" when it is not a guarded interpreter.
func interpreterLanguage(base string) string {
	if m := versionedInterpreter.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	switch base {
	case "node", "nodejs":
		return "node"
	case "deno":
		return "deno"
	}
	return ""
}

// interpreterSpec describes one interpreter's command line closely enough to find its code: short letters cluster, as in perl -lne.
type interpreterSpec struct {
	inlineLetters string
	// attached lets inline code follow its letter in the same argument, as in perl -e'print 1'.
	attached     bool
	valueLetters string
	fileLetters  string
	// stopLetters and stopLong mean the interpreter runs something other than a file or stdin, such as python -m or php -S.
	stopLetters string
	inlineLong  map[string]bool
	valueLong   map[string]bool
	stopLong    map[string]bool
	// repeat lets several inline flags add up to one program, as in perl -e 'a' -e 'b'.
	repeat bool
}

var interpreterSpecs = map[string]interpreterSpec{
	"python": {inlineLetters: "c", attached: true, valueLetters: "WXm", stopLetters: "m", valueLong: flagSet("--check-hash-based-pycs")},
	"node": {inlineLetters: "ep", valueLetters: "rC", inlineLong: flagSet("--eval", "--print"),
		valueLong: flagSet("--require", "--import", "--loader", "--experimental-loader", "--conditions", "--input-type", "--env-file", "--title", "--run"),
		stopLong:  flagSet("--run")},
	"php":  {inlineLetters: "r", attached: true, valueLetters: "cdztSBREF", fileLetters: "f", stopLetters: "S"},
	"perl": {inlineLetters: "eE", attached: true, valueLetters: "MmIxidDC", repeat: true},
	"ruby": {inlineLetters: "e", attached: true, valueLetters: "IrCEF", repeat: true},
}

var denoValueFlags = flagSet("-c", "--config", "--import-map", "--location", "--cert", "-L", "--log-level", "--seed", "--ext")

var denoScriptExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts"}

// interpreterRun is what an interpreter call executes: inline code, a script file, or code read from stdin.
type interpreterRun struct {
	Code  string
	File  string
	Stdin bool
}

func parseInterpreterArgs(lang string, args []string) interpreterRun {
	if lang == "deno" {
		return parseDenoArgs(args)
	}
	spec := interpreterSpecs[lang]
	var code []string
	file, stopped := "", false
	takeNext := func(i *int) (string, bool) {
		if *i+1 < len(args) {
			*i++
			return args[*i], true
		}
		return "", false
	}
scan:
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			if len(code) == 0 && i+1 < len(args) {
				file = args[i+1]
			}
			break scan
		case arg == "-":
			if len(code) == 0 {
				file = "-"
			}
			break scan
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg, "=")
			switch {
			case spec.inlineLong[name]:
				if !hasValue {
					value, hasValue = takeNext(&i)
				}
				if hasValue {
					code = append(code, value)
				}
				if !spec.repeat {
					break scan
				}
			case spec.valueLong[name] && !hasValue:
				i++
			}
			if spec.stopLong[name] {
				stopped = true
				break scan
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			for j := 1; j < len(arg); j++ {
				letter, rest := arg[j], arg[j+1:]
				switch {
				case strings.IndexByte(spec.inlineLetters, letter) >= 0:
					if spec.attached && rest != "" {
						code = append(code, rest)
					} else if next, ok := takeNext(&i); ok {
						code = append(code, next)
					}
					if !spec.repeat {
						break scan
					}
				case strings.IndexByte(spec.fileLetters, letter) >= 0:
					if rest != "" {
						file = rest
					} else {
						file, _ = takeNext(&i)
					}
					break scan
				case strings.IndexByte(spec.valueLetters, letter) >= 0:
					if rest == "" {
						i++
					}
					if strings.IndexByte(spec.stopLetters, letter) >= 0 {
						stopped = true
						break scan
					}
				default:
					continue
				}
				break
			}
		default:
			if len(code) == 0 {
				file = arg
			}
			break scan
		}
	}
	switch {
	case len(code) > 0:
		return interpreterRun{Code: strings.Join(code, "\n")}
	case file == "-":
		return interpreterRun{Stdin: true}
	case file != "":
		return interpreterRun{File: file}
	case stopped:
		return interpreterRun{}
	}
	return interpreterRun{Stdin: true}
}

func parseDenoArgs(args []string) interpreterRun {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			if denoValueFlags[arg] {
				i++
			}
			continue
		}
		positional = append(positional, arg)
		if len(positional) == 2 {
			break
		}
	}
	if len(positional) == 0 {
		return interpreterRun{}
	}
	switch positional[0] {
	case "eval":
		if len(positional) > 1 {
			return interpreterRun{Code: positional[1]}
		}
		return interpreterRun{}
	case "run":
		if len(positional) > 1 && positional[1] != "-" {
			return interpreterRun{File: positional[1]}
		}
		return interpreterRun{}
	}
	for _, ext := range denoScriptExtensions {
		if strings.HasSuffix(positional[0], ext) {
			return interpreterRun{File: positional[0]}
		}
	}
	return interpreterRun{}
}
