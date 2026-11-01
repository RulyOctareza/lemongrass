// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"strings"
	"testing"
)

func TestSimpleCommand(t *testing.T) {
	cases := []struct {
		name    string
		command string
		holds   string
	}{
		{"git push", "git push origin main", ""},
		{"assignment prefix", "GIT_TRACE=1 git push", ""},
		{"file redirect", "npm install > install.log 2>&1", ""},
		{"quoted args", `git commit -m "fix: login (1)"`, ""},
		{"line continuation", "docker pull \\\n  postgres:16", ""},
		{"timeout wrapper", "timeout 600 docker pull postgres:16", ""},
		{"block from ask 4cy9sqf6", "python3 - <<'PY'\nimport pathlib\npathlib.Path('a.ts').write_text('x')\nPY\nnpx prettier --write a.ts && npx vue-tsc --noEmit && npx eslint a.ts", "several commands"},
		{"two lines", "git add a.go\ngit commit -m x", "several commands"},
		{"semicolon", "cd ui; npm install", "several commands"},
		{"and chain", "npm ci && npm run build", "chained"},
		{"pipe", "curl -sL https://example.test/x | tar x", "pipeline"},
		{"heredoc", "cat <<EOF > a.txt\nhi\nEOF", "heredoc"},
		{"herestring", "git apply <<< \"$PATCH\"", "heredoc"},
		{"command substitution", "git push origin $(git branch --show-current)", "command substitution"},
		{"backticks", "git push origin `git branch --show-current`", "command substitution"},
		{"process substitution", "diff <(ls a) <(ls b)", "process substitution"},
		{"subshell", "(cd ui && npm install)", "subshell"},
		{"background", "npm run dev &", "background"},
		{"bash -c", `bash -c "npm install"`, "code run by bash"},
		{"python -c", `python3 -c "import os"`, "code run by python3"},
		{"versioned python", "python3.12 script.py", "code run by python3"},
		{"node -e", `node -e "1"`, "code run by node"},
		{"eval", "eval npm install", "code run by eval"},
		{"source", "source env.sh", "code run by source"},
		{"xargs", "xargs rm < list.txt", "code run by xargs"},
		{"find exec", "find . -name '*.o' -exec rm {} +", "find -exec"},
		{"wrapped interpreter", "env FOO=1 bash -c 'npm install'", "code run by bash"},
		{"computed name", "$TOOL install", "computed"},
		{"control flow", "for f in a b; do git add $f; done", "control flow"},
		{"function", "f() { git push; }", "function"},
		{"assignments only", "FOO=1", "only variable assignments"},
		{"unparseable", "git commit -m 'unterminated", "cannot be parsed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			holds, simple := simpleCommand(tc.command)
			if tc.holds == "" {
				if !simple {
					t.Fatalf("simpleCommand(%q) rejected it: %s", tc.command, holds)
				}
				return
			}
			if simple || !strings.Contains(holds, tc.holds) {
				t.Fatalf("simpleCommand(%q) = %q, %v, want a rejection holding %q", tc.command, holds, simple, tc.holds)
			}
		})
	}
}
