// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// These cases spell paths the Unix way; the Windows spellings are in the _windows_test.go files beside them.

//go:build !windows

package guard

import "testing"

func TestDynamicTarget(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{`rm -rf "$TMP/"*`, true},
		{`rm -rf $TMP/*`, true},
		{`rm -f -- "$FILE"`, true},
		{`rm -rf "${OUT}"`, true},
		{"rm -rf `pwd`/build", true},
		{`rm -rf $(ls -d build*)`, true},
		{`find "$D" -name '*.o' -delete`, true},
		{`find -L "$D" -delete`, true},
		{`chmod -R 777 "$X"`, true},
		{`chown -R me:me "$DIR"/sub`, true},
		{`chmod --recursive 755 $X`, true},
		{`shred -u "$F"`, true},
		{`unlink "$F"`, true},
		{`bash -c 'rm -rf "$TMP"/x'`, true},
		{`env FOO=1 rm -rf "$X"`, true},
		{`timeout 5 rm -rf "$X"`, true},
		{`D=build; rm -rf "$D/x"`, true},

		{`rm -rf build`, false},
		{`rm -rf build/{a,b}`, false},
		{`rm -rf ./*.log`, false},
		{`rm -rf "$HOME/x"`, false},
		{`rm -rf ${HOME}/x`, false},
		{`rm -rf ~/x`, false},
		{`chmod 644 "$F"`, false},
		{`chmod -R "$MODE" build`, false},
		{`chmod -R --reference=a.txt build`, false},
		{`find "$D" -name '*.o'`, false},
		{`find . -name "$PAT" -delete`, false},
		{`echo "$X" && rm -rf build`, false},
		{`cp -r "$SRC" build`, false},
		{`find . -name '*.o' | xargs rm -f`, false},
	}
	for _, tc := range cases {
		script, err := parseShell(tc.command)
		if err != nil {
			t.Fatalf("%q: %v", tc.command, err)
		}
		got := false
		for _, call := range script.Calls {
			if _, ok := dynamicTarget(call); ok {
				got = true
			}
		}
		if got != tc.want {
			t.Errorf("%q: dynamic target = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestDynamicTargetDetail(t *testing.T) {
	script, err := parseShell(`rm -rf "$TMP/"*`)
	if err != nil {
		t.Fatal(err)
	}
	detail, ok := dynamicTarget(script.Calls[0])
	if !ok || detail != "'$TMP/*' is built from a variable or substitution, so where rm lands is unknown until it runs. Write the path out." {
		t.Errorf("detail = %q", detail)
	}
}
