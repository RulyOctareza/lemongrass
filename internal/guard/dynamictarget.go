// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// dynamicTarget reports a destructive call whose target holds a parameter expansion or substitution, such as rm -rf "$TMP/"* with $TMP empty. It is judged apart from the deletion scope so it still holds when a policy allows rm-force. A variable assigned earlier in the same command is not resolved, and a leading $HOME or ${HOME} counts as written out, as in literalTarget.
func dynamicTarget(call shellCall) (string, bool) {
	if call.Indirect {
		return "", false
	}
	for _, i := range destructiveTargetIndexes(call) {
		// A Windows path the gate cannot pin down, such as %VAR% or an unknown short name, counts as dynamic; off Windows pathLiteral is always true.
		if (!dynamicAt(call.ArgDynamic, i) && pathLiteral(call.Args[i])) || literalTarget(call.Args[i]) {
			continue
		}
		return "'" + call.Args[i] + "' is built from a variable or substitution, so where " + call.Name + " lands is unknown until it runs. Write the path out.", true
	}
	return "", false
}

// destructiveTargetIndexes returns the argument indexes a destructive call acts on, or nil for a call that is not destructive.
func destructiveTargetIndexes(call shellCall) []int {
	switch call.Name {
	case "rm", "shred", "srm", "wipe", "unlink":
		return operandIndexes(call.Args, 0)
	case "find":
		if !containsArg(call.Args, "-delete") {
			return nil
		}
		return findRootIndexes(call.Args)
	case "chmod", "chown", "chgrp":
		if !hasShortFlag(call.Args, "R") && !hasLongFlag(call.Args, "--recursive") {
			return nil
		}
		skip := 1
		if hasLongFlag(call.Args, "--reference") {
			skip = 0
		}
		return operandIndexes(call.Args, skip)
	}
	return nil
}

// operandIndexes returns the indexes of the non-flag arguments after the first skip of them.
func operandIndexes(args []string, skip int) []int {
	var indexes []int
	flagsDone := false
	for i, arg := range args {
		if !flagsDone && arg == "--" {
			flagsDone = true
			continue
		}
		if !flagsDone && strings.HasPrefix(arg, "-") && len(arg) > 1 {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		indexes = append(indexes, i)
	}
	return indexes
}

func findRootIndexes(args []string) []int {
	i := 0
	for ; i < len(args); i++ {
		arg := args[i]
		if arg == "-D" {
			i++
		} else if arg != "-L" && arg != "-H" && arg != "-P" && !strings.HasPrefix(arg, "-O") {
			break
		}
	}
	var indexes []int
	for ; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") || arg == "(" || arg == "!" || arg == "," || arg == ")" {
			break
		}
		indexes = append(indexes, i)
	}
	return indexes
}
