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
