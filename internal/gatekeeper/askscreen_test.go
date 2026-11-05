// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"fmt"
	"strings"
	"testing"
)

// The streams are shaped like a ConPTY's: cursor positioning instead of spaces, repaints after a resize, rows padded
// with line feeds, erases.
func TestScreenTailReadsConPTYOutputAsLines(t *testing.T) {
	cases := []struct {
		name   string
		resize *askSize
		writes []string
		want   string
	}{
		{"cursor move stands for a space", nil,
			[]string{"Proceed?", "\x1b[1;10Hyes", "\r\ngot:yes\r\n"}, "Proceed? yes\ngot:yes"},
		{"repaint after a resize", &askSize{rows: 30, cols: 120},
			[]string{"\x1b[?25lName?\x1b[?25h", "\x1b[?25l\x1b[HName?\x1b[K" + strings.Repeat("\r\n\x1b[K", 28) + "\x1b[1;7H\x1b[?25h", "bob\r\nhi bob\r\n"},
			"Name? bob\nhi bob"},
		{"erase line and rewrite", nil, []string{"abc\x1b[2K\rxyz"}, "xyz"},
		{"clear screen", nil, []string{"old\r\n\x1b[2J\x1b[Hnew"}, "new"},
		{"colors dropped", nil, []string{"\x1b[1;32mok\x1b[0m\r\n"}, "ok"},
		{"erase characters", nil, []string{"hello\x1b[1G\x1b[2Xab"}, "abllo"},
		{"title string dropped", nil, []string{"\x1b]0;C:\\Windows\\bash.exe\x07ready"}, "ready"},
		{"utf8 kept", nil, []string{"caf\xc3", "\xa9 ok"}, "café ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScreenTail(askTailLines, askSize{rows: 24, cols: 100})
			for i, w := range c.writes {
				if i == 1 && c.resize != nil {
					s.resize(c.resize.rows, c.resize.cols)
				}
				s.Write([]byte(w))
			}
			if got := s.String(); got != c.want {
				t.Fatalf("tail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestScreenTailKeepsRowsThatScrolledOff(t *testing.T) {
	s := newScreenTail(askTailLines, askSize{rows: 24, cols: 100})
	var want []string
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(s, "line %d\r\n", i)
		want = append(want, fmt.Sprintf("line %d", i))
	}
	if got := s.String(); got != strings.Join(want, "\n") {
		t.Fatalf("tail = %q", got)
	}
}

func TestScreenTailKeepsTheLastLines(t *testing.T) {
	s := newScreenTail(3, askSize{rows: 2, cols: 20})
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(s, "l%d\r\n", i)
	}
	if got := s.String(); got != "l4\nl5\nl6" {
		t.Fatalf("tail = %q", got)
	}
}

func TestScreenTailWrapsAtTheLastColumn(t *testing.T) {
	s := newScreenTail(askTailLines, askSize{rows: 5, cols: 5})
	s.Write([]byte("abcdefg"))
	if got := s.String(); got != "abcde\nfg" {
		t.Fatalf("tail = %q", got)
	}
}
