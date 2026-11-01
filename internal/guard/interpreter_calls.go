// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

// destructiveCall is a standard-library call that deletes or moves a path. The tables are a fixed tripwire against accident: code that builds the call at runtime, aliases it or loads it through eval is not caught.
type destructiveCall struct {
	name string
	// paths is how many leading arguments are paths that must be literals inside the project, two for a move or rename, -1 when every argument is a path.
	paths int
	// receiver marks a method whose target is its receiver, as in pathlib's Path("x").unlink(); it passes only on a Path("<literal>") receiver.
	receiver bool
	// anyObject lets the name follow any object, as in fs.rmSync or fsp.rm, instead of only standing alone.
	anyObject bool
}

var destructiveCalls = map[string][]destructiveCall{
	"python": {
		{name: "shutil.rmtree", paths: 1}, {name: "rmtree", paths: 1},
		{name: "os.remove", paths: 1}, {name: "os.unlink", paths: 1}, {name: "os.rmdir", paths: 1}, {name: "os.removedirs", paths: 1},
		{name: "shutil.move", paths: 2}, {name: "os.rename", paths: 2}, {name: "os.replace", paths: 2}, {name: "os.renames", paths: 2},
		{name: "unlink", receiver: true}, {name: "rmdir", receiver: true},
	},
	"node": {
		{name: "rm", paths: 1, anyObject: true}, {name: "rmSync", paths: 1, anyObject: true},
		{name: "rmdir", paths: 1, anyObject: true}, {name: "rmdirSync", paths: 1, anyObject: true},
		{name: "unlink", paths: 1, anyObject: true}, {name: "unlinkSync", paths: 1, anyObject: true},
		{name: "rename", paths: 2, anyObject: true}, {name: "renameSync", paths: 2, anyObject: true},
		{name: "rimraf", paths: 1, anyObject: true}, {name: "rimrafSync", paths: 1, anyObject: true},
		{name: "remove", paths: 1, anyObject: true}, {name: "removeSync", paths: 1, anyObject: true},
		{name: "emptyDir", paths: 1, anyObject: true}, {name: "emptyDirSync", paths: 1, anyObject: true},
		{name: "move", paths: 2, anyObject: true}, {name: "moveSync", paths: 2, anyObject: true},
	},
	"deno": {
		{name: "Deno.remove", paths: 1}, {name: "Deno.removeSync", paths: 1},
		{name: "Deno.rename", paths: 2}, {name: "Deno.renameSync", paths: 2},
	},
	"php": {
		{name: "unlink", paths: 1}, {name: "rmdir", paths: 1}, {name: "rename", paths: 2},
	},
	"perl": {
		{name: "unlink", paths: -1}, {name: "rmdir", paths: -1}, {name: "rmtree", paths: -1}, {name: "remove_tree", paths: -1},
		{name: "move", paths: 2}, {name: "rename", paths: 2},
	},
	"ruby": {
		{name: "FileUtils.rm", paths: 1}, {name: "FileUtils.rm_f", paths: 1}, {name: "FileUtils.rm_r", paths: 1}, {name: "FileUtils.rm_rf", paths: 1},
		{name: "FileUtils.rmtree", paths: 1}, {name: "FileUtils.remove", paths: 1}, {name: "FileUtils.remove_dir", paths: 1},
		{name: "FileUtils.remove_entry", paths: 1}, {name: "FileUtils.remove_entry_secure", paths: 1}, {name: "FileUtils.mv", paths: 2},
		{name: "rm_f", paths: 1}, {name: "rm_r", paths: 1}, {name: "rm_rf", paths: 1}, {name: "remove_dir", paths: 1}, {name: "remove_entry", paths: 1},
		{name: "File.delete", paths: 1}, {name: "File.unlink", paths: 1}, {name: "File.rename", paths: 2},
		{name: "Dir.rmdir", paths: 1}, {name: "Dir.delete", paths: 1}, {name: "Dir.unlink", paths: 1},
	},
}

// parenOptional marks languages that call a function without parentheses, as in perl's unlink "x".
var parenOptional = map[string]bool{"perl": true, "ruby": true}

// namedByString marks languages that pass a function by its name in a string, as in PHP's array_map('unlink', $files).
var namedByString = map[string]bool{"php": true}
