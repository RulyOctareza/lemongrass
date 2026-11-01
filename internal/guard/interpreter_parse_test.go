// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"reflect"
	"testing"
)

func TestShellPrograms(t *testing.T) {
	type prog struct{ Lang, Code, File string }
	cases := []struct {
		command string
		want    []prog
	}{
		{`python3 -c 'import os; os.remove("x")'`, []prog{{"python", `import os; os.remove("x")`, ""}}},
		{`python -Bc 'print(1)'`, []prog{{"python", "print(1)", ""}}},
		{`python3.12 -c"print(1)" arg`, []prog{{"python", "print(1)", ""}}},
		{`python3 -m pytest -x`, nil},
		{`python3 -W ignore script.py`, []prog{{"python", "", "script.py"}}},
		{`python3 manage.py migrate`, []prog{{"python", "", "manage.py"}}},
		{`python3 - <<'EOF'` + "\nimport shutil\nshutil.rmtree(p)\nEOF", []prog{{"python", "import shutil\nshutil.rmtree(p)\n", ""}}},
		{`python3 <<< 'print(1)'`, []prog{{"python", "print(1)", ""}}},
		{`python3 < clean.py`, []prog{{"python", "", "clean.py"}}},
		{`python3 script.py <<EOF` + "\ndata\nEOF", []prog{{"python", "", "script.py"}}},
		{`echo 'import os' | python3`, []prog{{"python", "import os", ""}}},
		{`printf 'a\nb %s\n' c | python3 -`, []prog{{"python", "a\nb c\n", ""}}},
		{"cat <<EOF | python3\nprint(1)\nEOF", []prog{{"python", "print(1)\n", ""}}},
		{`cat clean.py | python3`, []prog{{"python", "", "clean.py"}}},
		{`echo hi | python3 script.py`, []prog{{"python", "", "script.py"}}},
		{`node -e 'fs.rmSync(d)'`, []prog{{"node", "fs.rmSync(d)", ""}}},
		{`node -pe 'process.cwd()'`, []prog{{"node", "process.cwd()", ""}}},
		{`node --eval='x()'`, []prog{{"node", "x()", ""}}},
		{`node -r dotenv/config build.js`, []prog{{"node", "", "build.js"}}},
		{`node --run build`, nil},
		{`php -r 'unlink("a");'`, []prog{{"php", `unlink("a");`, ""}}},
		{`php -d memory_limit=1G artisan migrate`, []prog{{"php", "", "artisan"}}},
		{`php -f clean.php`, []prog{{"php", "", "clean.php"}}},
		{`php -S localhost:8000`, nil},
		{`perl -e 'unlink $f' -e 'print 1'`, []prog{{"perl", "unlink $f\nprint 1", ""}}},
		{`perl -i.bak -pe 's/a/b/' file.txt`, []prog{{"perl", "s/a/b/", ""}}},
		{`perl -e'print 1'`, []prog{{"perl", "print 1", ""}}},
		{`perl -MFile::Path -e 'rmtree("x")'`, []prog{{"perl", `rmtree("x")`, ""}}},
		{`ruby -e 'FileUtils.rm_rf(d)'`, []prog{{"ruby", "FileUtils.rm_rf(d)", ""}}},
		{`ruby -I lib -r json clean.rb`, []prog{{"ruby", "", "clean.rb"}}},
		{`deno eval 'Deno.removeSync(p)'`, []prog{{"deno", "Deno.removeSync(p)", ""}}},
		{`deno run -A --config deno.json main.ts`, []prog{{"deno", "", "main.ts"}}},
		{`deno main.ts`, []prog{{"deno", "", "main.ts"}}},
		{`deno test`, nil},
		{`bash -c 'python3 -c "print(1)"'`, []prog{{"python", "print(1)", ""}}},
		{`timeout 5 python3 -c 'print(1)'`, []prog{{"python", "print(1)", ""}}},
		{`env FOO=1 python3 <<< 'print(1)'`, []prog{{"python", "print(1)", ""}}},
		{`bash cleanup.sh`, []prog{{"sh", "", "cleanup.sh"}}},
		{`sh -x -o errexit cleanup.sh arg`, []prog{{"sh", "", "cleanup.sh"}}},
		{`bash -s < cleanup.sh`, []prog{{"sh", "", "cleanup.sh"}}},
		{`bash < cleanup.sh`, []prog{{"sh", "", "cleanup.sh"}}},
		{`cat cleanup.sh | bash`, []prog{{"sh", "", "cleanup.sh"}}},
		{`source .venv/bin/activate`, []prog{{"sh", "", ".venv/bin/activate"}}},
		{`. ./env.sh`, []prog{{"sh", "", "./env.sh"}}},
		{`./gradlew build`, []prog{{"", "", "./gradlew"}}},
		{`scripts/clean`, []prog{{"", "", "scripts/clean"}}},
		{`gradle build`, nil},
		{`echo 'rm -rf x' | bash`, nil},
	}
	for _, tc := range cases {
		script, err := parseShell(tc.command)
		if err != nil {
			t.Fatalf("%q: %v", tc.command, err)
		}
		var got []prog
		for _, p := range script.Programs {
			got = append(got, prog{p.Lang, p.Code, p.File})
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: programs = %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestPipedShellTextIsParsed(t *testing.T) {
	for _, command := range []string{
		`echo 'rm -rf build' | bash`,
		`printf 'rm -rf %s\n' build | sh`,
		"cat <<EOF | bash\nrm -rf build\nEOF",
		`echo -e 'echo hi\nrm -rf build' | bash -s`,
	} {
		script, err := parseShell(command)
		if err != nil {
			t.Fatalf("%q: %v", command, err)
		}
		found := false
		for _, call := range script.Calls {
			if call.Name == "rm" && reflect.DeepEqual(call.Args, []string{"-rf", "build"}) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: piped rm not parsed, calls %+v", command, script.Calls)
		}
	}
}

func TestProgramAt(t *testing.T) {
	script, err := parseShell(`cd sub && python3 clean.py`)
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Programs) != 1 || script.Programs[0].At != 2 || script.Calls[0].Name != "cd" {
		t.Errorf("programs %+v, calls %+v", script.Programs, script.Calls)
	}
}
