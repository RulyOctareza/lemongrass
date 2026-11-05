// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

var deletionRules = []rule{
	{ID: "rm-force", Category: catDeletion, Description: "rm with a force or recursive flag", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"rm"}, Flags: []string{"-f", "-r", "-R", "--force", "--recursive", "--no-preserve-root"}},
	}},
	{ID: "rm-plain", Category: catDeletion, Description: "removing a file", Mode: ModeAsk, Forms: []form{{Tools: []string{"rm"}}}},
	{ID: "find-delete", Category: catDeletion, Description: "find -delete", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"find"}, Args: []string{"-delete"}},
	}},
	{ID: "shred", Category: catDeletion, Description: "overwriting files unrecoverably", Mode: ModeDeny, Forms: []form{{Tools: []string{"shred", "srm", "wipe"}}}},
	{ID: "unlink", Category: catDeletion, Description: "unlinking a file", Mode: ModeDeny, Forms: []form{{Tools: []string{"unlink"}}}},
	// cmd's del and rd and PowerShell's Remove-Item reach the gate inside a command it otherwise denies, or as a mangled parse; a recursive or forced one is denied here too, since no deletion scope applies to them.
	{ID: "windows-delete", Category: catDeletion, Description: "a recursive or forced delete with del, rd or Remove-Item", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"del", "erase", "rd", "rmdir"}, Match: winOptionForm("s", "q", "f")},
		{Tools: []string{"remove-item", "ri"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			return matched(psParameter(call.Args, "recurse", 1) != "" || psParameter(call.Args, "force", 2) != "")
		}},
	}},
	{ID: "dynamic-target", Category: catDeletion, Description: DynamicTargetDescription, Mode: ModeDeny, Forms: []form{
		{Match: func(call shellCall, _ matchEnv) (string, bool) { return dynamicTarget(call) }},
	}},
	{ID: "interpreter-delete", Category: catDeletion, Description: InterpreterDeleteDescription, Mode: ModeDeny},
	{ID: "file-truncate", Category: catDeletion, Description: "truncating a file", Mode: ModeAsk, Forms: []form{{Tools: []string{"truncate"}}}},
}

// scriptRules are judged by the interpreters checks, which read a script file at judge time; they sit in one category so script scanning is switched in one place.
var scriptRules = []rule{
	{ID: "script-delete", Category: catScripts, Description: ScriptDeleteDescription, Mode: ModeDeny},
	{ID: "script-command", Category: catScripts, Description: ScriptCommandDescription, Mode: ModeDeny},
}
