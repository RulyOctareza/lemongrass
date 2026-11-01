// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"reflect"
	"testing"
)

func TestSubprocessCommands(t *testing.T) {
	cases := []struct {
		lang, code string
		want       []string
	}{
		{"python", `os.system("make clean")`, []string{"make clean"}},
		{"python", `subprocess.run(["rm", "-rf", "it's"], check=True)`, []string{`'rm' '-rf' 'it'\''s'`}},
		{"python", `subprocess.run(["rm", "-rf", d])`, nil},
		{"python", `subprocess.run("rm -rf " + d, shell=True)`, nil},
		{"python", `subprocess.check_output(f"rm {d}", shell=True)`, nil},
		{"python", `subprocess.Popen(
    ["git", "push"]
)`, []string{`'git' 'push'`}},
		{"node", `require("child_process").execSync("npm test", {stdio: "inherit"})`, []string{"npm test"}},
		{"node", `cp.spawn("git", ["push", "--force"])`, []string{`'git' 'push' '--force'`}},
		{"node", "exec(`rm -rf ${dir}`)", nil},
		{"node", `/x/.exec(line)`, nil},
		{"deno", `new Deno.Command("git", { args: ["push"] })`, []string{`'git' 'push'`}},
		{"deno", `new Deno.Command("ls")`, []string{"ls"}},
		{"php", "echo `ls -la`;", []string{"ls -la"}},
		{"php", "echo `rm $f`;", nil},
		{"php", `passthru('make');`, []string{"make"}},
		{"perl", `system "rm", "-rf", "x";`, []string{`'rm' '-rf' 'x'`}},
		{"perl", `my $o = qx{git status};`, []string{"git status"}},
		{"perl", `system("rm -rf $d")`, nil},
		{"ruby", `system("make", "clean")`, []string{`'make' 'clean'`}},
		{"ruby", `out = %x(git log)`, []string{"git log"}},
		{"ruby", "puts `rm -rf #{d}`", nil},
	}
	for _, tc := range cases {
		if got := subprocessCommands(tc.lang, tc.code); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %q: commands %q, want %q", tc.lang, tc.code, got, tc.want)
		}
	}
}
