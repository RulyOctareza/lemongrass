// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// These cases spell paths the Unix way; the Windows spellings are in the _windows_test.go files beside them.

//go:build !windows

package guard

import (
	"path/filepath"
	"strings"
	"testing"
)

func decideIn(t *testing.T, root, home, cwd, command string, policy Policy) *Verdict {
	t.Helper()
	return Decide(Input{Tool: "Bash", Command: command, Cwd: cwd, Home: home, ProjectRoot: root}, policy)
}

func TestInterpreterInlineCode(t *testing.T) {
	root, home, _ := rmProject(t)
	cases := []struct {
		command string
		want    string
	}{
		{`python3 -c 'print(1)'`, ""},
		{`python3 -c 'import shutil; shutil.rmtree("build/out")'`, ""},
		{`python3 -c 'import shutil; shutil.rmtree(p)'`, "interpreter-delete"},
		{`python3 -c 'import shutil; shutil.rmtree("/etc")'`, "interpreter-delete"},
		{`python3 -c 'import os; os.remove(".git/index.lock")'`, "interpreter-delete"},
		{`python3 -c 'import os; os.remove("biblio/books/toc.md")'`, "interpreter-delete"},
		{`python3 -c 'import os; os.remove("a" + x)'`, "interpreter-delete"},
		{`python3 -c 'import os; os.remove(f"{d}/a")'`, "interpreter-delete"},
		{`python3 -c 'import os; os.remove(r"a.log")'`, ""},
		{"python3 -c 'import shutil\nshutil.rmtree(\n    \"build/out\",\n)'", ""},
		{`python3 -c 'from pathlib import Path; Path("a.log").unlink()'`, ""},
		{`python3 -c 'from pathlib import Path; Path("/etc/x").unlink()'`, "interpreter-delete"},
		{`python3 -c 'from pathlib import Path; p.unlink()'`, "interpreter-delete"},
		{`python3 -c 'import os; list(map(os.remove, files))'`, "interpreter-delete"},
		{`python3 -c 'import shutil; shutil.move("a.log", "/tmp/x")'`, "interpreter-delete"},
		{`python3 -c 'import shutil; shutil.move("a.log", "sub/a.log")'`, ""},
		{`python3 -c 'print(open(os.path.expanduser("~/.ssh/id_rsa")).read())'`, "secret-path"},
		{`python3 -c 'print((Path.home() / ".aws" / "credentials").read_text())'`, "secret-path"},
		{"python3 <<'EOF'\nimport shutil\nshutil.rmtree(target)\nEOF", "interpreter-delete"},
		{"python3 - <<EOF\nimport os\nos.remove('a.log')\nEOF", ""},
		{`echo 'import os; os.unlink(x)' | python3`, "interpreter-delete"},
		{`python3 <<< 'import os; os.unlink(x)'`, "interpreter-delete"},
		{`timeout 9 python3 -c 'import os; os.unlink(x)'`, "interpreter-delete"},
		{`bash -c "python3 -c 'import os; os.unlink(x)'"`, "interpreter-delete"},
		{`cd sub && python3 -c 'import os; os.remove("c.txt")'`, ""},
		{`cd "$X" && python3 -c 'import os; os.remove("c.txt")'`, "interpreter-delete"},
		{`python3 -c 'import os; os.system("rm -rf /etc")'`, "rm-force"},
		{`python3 -c 'import os; os.system("rm -rf build")'`, ""},
		{`python3 -c 'import subprocess; subprocess.run(["rm", "-rf", "/"])'`, "rm-force"},
		{`python3 -c 'import subprocess; subprocess.run("git push --force", shell=True)'`, "git-push-force"},
		{`python3 -c 'import subprocess; subprocess.run(["rm", "-rf", d])'`, ""},

		{`node -e 'require("fs").rmSync("build", {recursive: true})'`, ""},
		{`node -e 'fs.rmSync(path.join(__dirname, "dist"), {recursive: true})'`, "interpreter-delete"},
		{"node -e 'fs.rmSync(`build/${x}`)'", "interpreter-delete"},
		{"node -e 'fs.rmSync(`build/out`)'", ""},
		{`node -e 'fs.promises.rm("/home")'`, "interpreter-delete"},
		{`node -e 'const { rmSync, existsSync } = require("fs")'`, ""},
		{`node -e 'files.forEach(fs.unlinkSync)'`, "interpreter-delete"},
		{`node -e 'el.remove()'`, ""},
		{`node -e 'require("child_process").execSync("rm -rf ~")'`, "rm-force"},
		{`node -e 'spawn("rm", ["-rf", "/etc"])'`, "rm-force"},
		{`node -p 'require("fs").readFileSync(require("os").homedir() + "/.ssh/id_rsa", "utf8")'`, "secret-path"},

		{`php -r 'unlink("a.log");'`, ""},
		{`php -r 'unlink($f);'`, "interpreter-delete"},
		{`php -r 'unlink("$dir/a");'`, "interpreter-delete"},
		{`php -r 'array_map("unlink", glob("*.log"));'`, "interpreter-delete"},
		{`php -r 'echo shell_exec("rm -rf /etc");'`, "rm-force"},

		{`perl -e 'unlink "a.log", "b.log"'`, ""},
		{`perl -e 'unlink "a.log", $f'`, "interpreter-delete"},
		{`perl -e 'unlink glob "*.log"'`, "interpreter-delete"},
		{`perl -e 'unlink;'`, "interpreter-delete"},
		{`perl -pe 's/a/b/' a.log`, ""},
		{`perl -e 'system("rm", "-rf", "/etc")'`, "rm-force"},
		{`perl -e 'print qx(rm -rf /etc)'`, "rm-force"},

		{`ruby -e 'FileUtils.rm_rf("build")'`, ""},
		{`ruby -e 'FileUtils.rm_rf(dir)'`, "interpreter-delete"},
		{`ruby -e 'File.delete "#{d}/a"'`, "interpreter-delete"},
		{"ruby -e 'puts `rm -rf /etc`'", "rm-force"},

		{`deno eval 'Deno.removeSync("build", {recursive: true})'`, ""},
		{`deno eval 'Deno.removeSync(p)'`, "interpreter-delete"},
		{`deno eval 'new Deno.Command("rm", {args: ["-rf", "/etc"]}).outputSync()'`, "rm-force"},
	}
	for _, tc := range cases {
		if got := ruleOf(decideIn(t, root, home, root, tc.command, Policy{})); got != tc.want {
			t.Errorf("%q: rule %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestInterpreterNoProject(t *testing.T) {
	home := t.TempDir()
	got := decideIn(t, "", home, t.TempDir(), `python3 -c 'import os; os.remove("a.log")'`, Policy{})
	if ruleOf(got) != "interpreter-delete" {
		t.Errorf("rule %q, want interpreter-delete", ruleOf(got))
	}
}

func TestInterpreterDetail(t *testing.T) {
	root, home, _ := rmProject(t)
	got := decideIn(t, root, home, root, `python3 -c 'import shutil; shutil.rmtree(target)'`, Policy{})
	want := "In Python code, shutil.rmtree(target) takes a variable or expression, so where it lands is unknown until it runs. Write the path out as a string literal inside the project."
	if got == nil || got.Detail != want {
		t.Errorf("verdict %+v", got)
	}
	got = decideIn(t, root, home, root, `node -e 'fs.rmSync("/etc")'`, Policy{})
	if got == nil || !strings.Contains(got.Detail, `rmSync("/etc")`) || !strings.Contains(got.Detail, "outside the project "+root) {
		t.Errorf("verdict %+v", got)
	}
}

func TestInterpreterPolicy(t *testing.T) {
	root, home, _ := rmProject(t)
	command := `python3 -c 'import shutil; shutil.rmtree(p)'`
	if got := decideIn(t, root, home, root, command, Policy{Rules: map[string]string{"interpreter-delete": "allow"}}); got != nil {
		t.Errorf("allowed interpreter-delete still fires: %+v", got)
	}
	secret := `python3 -c 'open("/x/.ssh/id_rsa")'`
	if got := decideIn(t, root, home, filepath.Join(root, "sub"), secret, Policy{Rules: map[string]string{"interpreter-delete": "allow"}}); ruleOf(got) != "secret-path" {
		t.Errorf("secret path in code: %+v", got)
	}
}
