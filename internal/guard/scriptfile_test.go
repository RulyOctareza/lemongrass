// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// These cases spell paths the Unix way; the Windows spellings are in the _windows_test.go files beside them.

//go:build !windows

package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestScriptFiles(t *testing.T) {
	root, home, _ := rmProject(t)
	scripts := map[string]string{
		"clean.py":        "import shutil\nshutil.rmtree(target)\n",
		"tidy.py":         "import os\nos.remove('a.log')\n",
		"shell_out.py":    "import os\nos.system('rm -rf /etc')\n",
		"keys.py":         "print(open('/x/.ssh/id_rsa').read())\n",
		"clean.js":        "require('fs').rmSync(dir, {recursive: true})\n",
		"clean.sh":        "#!/bin/sh\nrm -rf \"$TMP\"/*\n",
		"tidy.sh":         "rm -f a.log\n",
		"computed.sh":     "#!/bin/sh\nexec \"$JAVACMD\" -jar app.jar \"$@\"\n",
		"broken.sh":       "if then ( fi\n",
		"keys.sh":         "cat ~/.ssh/id_rsa\n",
		"self.sh":         "bash self.sh\n",
		"inner.sh":        "python3 -c 'import os; os.unlink(x)'\n",
		"run":             "#!/usr/bin/env python3\nimport os\nos.remove(p)\n",
		"gradlew":         "#!/bin/sh\nexec \"$JAVACMD\" \"$@\"\n",
		"plain":           "rm -rf /etc\n",
		"sub/nested.sh":   "rm -rf /etc\n",
		"outer.sh":        "bash sub/nested.sh\n",
		"big.py":          "import shutil\nshutil.rmtree(target)\n" + strings.Repeat("#", maxScriptBytes),
		"envshebang":      "#!/usr/bin/env -S bash -e\nrm -rf /etc\n",
		"pushes.sh":       "git push --force\n",
		"rubyclean.rb":    "FileUtils.rm_rf(dir)\n",
		"perlclean.pl":    "unlink $f;\n",
		"phpclean.php":    "<?php unlink($f);\n",
		"denoclean.ts":    "Deno.removeSync(p)\n",
		"denotidy.ts":     "Deno.removeSync('a.log')\n",
		"sub/relative.py": "import os\nos.remove('../a.log')\n",
	}
	for name, text := range scripts {
		writeScript(t, filepath.Join(root, name), text)
	}
	cases := []struct {
		command string
		want    string
	}{
		{"python3 clean.py", "script-delete"},
		{"python3 tidy.py", ""},
		{"python3 shell_out.py", "script-command"},
		{"python3 keys.py", "secret-path"},
		{"node clean.js", "script-delete"},
		{"bash clean.sh", "script-command"},
		{"sh -e clean.sh", "script-command"},
		{"source clean.sh", "script-command"},
		{". ./clean.sh", "script-command"},
		{"bash < clean.sh", "script-command"},
		{"cat clean.sh | bash", "script-command"},
		{"./clean.sh", "script-command"},
		{"bash tidy.sh", ""},
		{"bash computed.sh", ""},
		{"bash broken.sh", ""},
		{"bash keys.sh", "secret-path"},
		{"bash self.sh", ""},
		{"bash inner.sh", "script-delete"},
		{"./run", "script-delete"},
		{"./gradlew build", ""},
		{"./plain", ""},
		{"bash plain", "script-command"},
		{"bash outer.sh", "script-command"},
		{"python3 big.py", ""},
		{"python3 missing.py", ""},
		{"./envshebang", "script-command"},
		{"bash pushes.sh", "script-command"},
		{"cat clean.py | python3", "script-delete"},
		{"python3 < clean.py", "script-delete"},
		{"ruby rubyclean.rb", "script-delete"},
		{"perl perlclean.pl", "script-delete"},
		{"php phpclean.php", "script-delete"},
		{"deno run -A denoclean.ts", "script-delete"},
		{"deno run denotidy.ts", ""},
		{"cd sub && python3 relative.py", ""},
		{"python3 sub/relative.py", "script-delete"},
		{`cd "$X" && python3 clean.py`, ""},
	}
	for _, tc := range cases {
		if got := ruleOf(decideIn(t, root, home, root, tc.command, Policy{})); got != tc.want {
			t.Errorf("%q: rule %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestScriptFileDetail(t *testing.T) {
	root, home, _ := rmProject(t)
	writeScript(t, filepath.Join(root, "clean.sh"), "rm -rf \"$TMP\"/*\n")
	got := decideIn(t, root, home, root, "bash clean.sh", Policy{})
	if got == nil || !strings.HasPrefix(got.Detail, "In script clean.sh, rule rm-force (rm with a force or recursive flag): '$TMP/*' uses a variable") {
		t.Errorf("verdict %+v", got)
	}
	writeScript(t, filepath.Join(root, "clean.py"), "import shutil\nshutil.rmtree(target)\n")
	got = decideIn(t, root, home, root, "python3 clean.py", Policy{})
	if got == nil || !strings.HasPrefix(got.Detail, "In script clean.py, shutil.rmtree(target) takes a variable") {
		t.Errorf("verdict %+v", got)
	}
}

func TestScriptFilePolicy(t *testing.T) {
	root, home, _ := rmProject(t)
	writeScript(t, filepath.Join(root, "push.sh"), "git push --force\n")
	writeScript(t, filepath.Join(root, "add.sh"), "git add -A\n")
	writeScript(t, filepath.Join(root, "keys.sh"), "cat ~/.ssh/id_rsa\n")
	writeScript(t, filepath.Join(root, "clean.py"), "import shutil\nshutil.rmtree(target)\n")
	cases := []struct {
		command string
		rules   map[string]string
		want    string
		mode    Mode
	}{
		{"bash push.sh", map[string]string{"git-push-force": "allow", "git-push": "allow"}, "", ModeDeny},
		{"bash push.sh", map[string]string{"git-push-force": "allow"}, "script-command", ModeAsk},
		{"bash push.sh", map[string]string{"script-command": "allow"}, "", ModeDeny},
		{"bash push.sh", nil, "script-command", ModeDeny},
		{"bash add.sh", map[string]string{"git-add": "ask"}, "script-command", ModeAsk},
		{"bash add.sh", map[string]string{"script-command": "deny"}, "script-command", ModeDeny},
		{"bash keys.sh", map[string]string{"script-command": "allow"}, "secret-path", ModeDeny},
		{"python3 clean.py", map[string]string{"script-delete": "allow"}, "", ModeDeny},
		{"python3 clean.py", map[string]string{"interpreter-delete": "allow"}, "script-delete", ModeDeny},
	}
	for _, tc := range cases {
		got := decideIn(t, root, home, root, tc.command, Policy{Rules: tc.rules})
		if ruleOf(got) != tc.want || (got != nil && got.Mode != tc.mode) {
			t.Errorf("%q with %v: verdict %+v, want %q mode %v", tc.command, tc.rules, got, tc.want, tc.mode)
		}
	}
}
