// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// screenTail is the ask tail for a terminal that sends screen updates rather than a stream of lines, which is what a
// Windows ConPTY does: it repaints, moves the cursor and pads rows instead of writing each line once. It keeps a small
// screen of the terminal's size, applies the cursor and erase sequences ConPTY uses, and reads the tail as the rows
// that scrolled off followed by the screen's rows down to the last one holding text, so the card and the model get
// the same lines a pty gives on Linux.
type screenTail struct {
	mu       sync.Mutex
	max      int
	rows     int
	cols     int
	grid     [][]rune
	row, col int
	// wrap is set after a write to the last column; the next printable rune moves to the next row first.
	wrap      bool
	scrolled  []string
	esc       escState
	params    []byte
	private   bool
	utf8Buf   []byte
	savedRow  int
	savedCol  int
	hasSaved  bool
	lastPrint rune
}

func newScreenTail(maxLines int, size askSize) *screenTail {
	s := &screenTail{max: maxLines}
	s.resize(size.rows, size.cols)
	return s
}

func (s *screenTail) resize(rows, cols int) {
	if rows < 1 || cols < 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	grid := make([][]rune, rows)
	for r := range grid {
		grid[r] = blankRow(cols)
		if r < len(s.grid) {
			copy(grid[r], s.grid[r])
		}
	}
	s.grid, s.rows, s.cols = grid, rows, cols
	s.row = min(s.row, rows-1)
	s.col = min(s.col, cols-1)
	s.wrap = false
}

func blankRow(cols int) []rune {
	row := make([]rune, cols)
	for i := range row {
		row[i] = ' '
	}
	return row
}

func (s *screenTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range p {
		s.feed(c)
	}
	return len(p), nil
}

func (s *screenTail) feed(c byte) {
	switch s.esc {
	case escStart:
		switch {
		case c == '[':
			s.esc, s.params, s.private = escCSI, s.params[:0], false
		case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
			s.esc = escString
		case c == '7':
			s.savedRow, s.savedCol, s.hasSaved = s.row, s.col, true
			s.esc = escNone
		case c == '8':
			s.restoreCursor()
			s.esc = escNone
		case c >= 0x20 && c <= 0x2f:
			s.esc = escIntermediate
		default:
			s.esc = escNone
		}
		return
	case escIntermediate:
		if c >= 0x30 && c <= 0x7e {
			s.esc = escNone
		}
		return
	case escCSI:
		switch {
		case c >= 0x40 && c <= 0x7e:
			s.esc = escNone
			s.csi(c)
		case c == '?' || c == '>' || c == '=':
			s.private = true
		case len(s.params) < 32:
			s.params = append(s.params, c)
		}
		return
	case escString:
		if c == 0x07 {
			s.esc = escNone
		} else if c == 0x1b {
			s.esc = escStringEsc
		}
		return
	case escStringEsc:
		if c == '\\' {
			s.esc = escNone
		} else if c != 0x1b {
			s.esc = escString
		}
		return
	}
	switch {
	case c == 0x1b:
		s.esc = escStart
	case c == '\r':
		s.col, s.wrap = 0, false
	case c == '\n':
		s.lineFeed()
	case c == 0x08:
		if s.col > 0 {
			s.col--
		}
		s.wrap = false
	case c == '\t':
		s.col = min((s.col/8+1)*8, s.cols-1)
	case c < 0x20 || c == 0x7f:
	case c < 0x80:
		s.print(rune(c))
	default:
		s.utf8Buf = append(s.utf8Buf, c)
		if utf8.FullRune(s.utf8Buf) {
			r, _ := utf8.DecodeRune(s.utf8Buf)
			s.utf8Buf = s.utf8Buf[:0]
			s.print(r)
		} else if len(s.utf8Buf) >= utf8.UTFMax {
			s.utf8Buf = s.utf8Buf[:0]
		}
	}
}

func (s *screenTail) print(r rune) {
	if s.wrap {
		s.col = 0
		s.lineFeed()
		s.wrap = false
	}
	s.grid[s.row][s.col] = r
	s.lastPrint = r
	if s.col == s.cols-1 {
		s.wrap = true
	} else {
		s.col++
	}
}

// lineFeed moves down a row, scrolling the top row into the tail at the bottom.
func (s *screenTail) lineFeed() {
	s.wrap = false
	if s.row < s.rows-1 {
		s.row++
		return
	}
	s.scrollUp(1)
}

func (s *screenTail) scrollUp(n int) {
	for range min(n, s.rows) {
		s.scrolled = append(s.scrolled, rowText(s.grid[0]))
		if len(s.scrolled) > s.max {
			s.scrolled = s.scrolled[len(s.scrolled)-s.max:]
		}
		copy(s.grid, s.grid[1:])
		s.grid[s.rows-1] = blankRow(s.cols)
	}
}

func (s *screenTail) restoreCursor() {
	if s.hasSaved {
		s.row, s.col = min(s.savedRow, s.rows-1), min(s.savedCol, s.cols-1)
	}
	s.wrap = false
}

// param is the n-th numeric CSI parameter, def when missing or zero.
func (s *screenTail) param(n, def int) int {
	fields := strings.Split(string(s.params), ";")
	if n >= len(fields) {
		return def
	}
	v, err := strconv.Atoi(fields[n])
	if err != nil || v == 0 {
		return def
	}
	return v
}

func (s *screenTail) csi(final byte) {
	if s.private {
		return
	}
	clampRow := func(r int) int { return max(0, min(r, s.rows-1)) }
	clampCol := func(c int) int { return max(0, min(c, s.cols-1)) }
	switch final {
	case 'H', 'f':
		s.row, s.col = clampRow(s.param(0, 1)-1), clampCol(s.param(1, 1)-1)
	case 'G', '`':
		s.col = clampCol(s.param(0, 1) - 1)
	case 'd':
		s.row = clampRow(s.param(0, 1) - 1)
	case 'A':
		s.row = clampRow(s.row - s.param(0, 1))
	case 'B', 'e':
		s.row = clampRow(s.row + s.param(0, 1))
	case 'C', 'a':
		s.col = clampCol(s.col + s.param(0, 1))
	case 'D':
		s.col = clampCol(s.col - s.param(0, 1))
	case 'E':
		s.row, s.col = clampRow(s.row+s.param(0, 1)), 0
	case 'F':
		s.row, s.col = clampRow(s.row-s.param(0, 1)), 0
	case 'K':
		s.eraseLine(s.param(0, 0))
	case 'J':
		s.eraseScreen(s.param(0, 0))
	case 'X':
		for c := s.col; c < min(s.col+s.param(0, 1), s.cols); c++ {
			s.grid[s.row][c] = ' '
		}
	case 'P':
		n := min(s.param(0, 1), s.cols-s.col)
		line := s.grid[s.row]
		copy(line[s.col:], line[s.col+n:])
		for c := s.cols - n; c < s.cols; c++ {
			line[c] = ' '
		}
	case '@':
		n := min(s.param(0, 1), s.cols-s.col)
		line := s.grid[s.row]
		copy(line[s.col+n:], line[s.col:s.cols-n])
		for c := s.col; c < s.col+n; c++ {
			line[c] = ' '
		}
	case 'S':
		s.scrollUp(s.param(0, 1))
	case 'b':
		for range min(s.param(0, 1), s.cols) {
			s.print(s.lastPrint)
		}
	case 's':
		s.savedRow, s.savedCol, s.hasSaved = s.row, s.col, true
	case 'u':
		s.restoreCursor()
	default:
		return
	}
	if final != 'b' {
		s.wrap = false
	}
}

func (s *screenTail) eraseLine(mode int) {
	line := s.grid[s.row]
	from, to := s.col, s.cols
	switch mode {
	case 1:
		from, to = 0, s.col+1
	case 2:
		from = 0
	}
	for c := from; c < min(to, s.cols); c++ {
		line[c] = ' '
	}
}

func (s *screenTail) eraseScreen(mode int) {
	switch mode {
	case 0:
		s.eraseLine(0)
		for r := s.row + 1; r < s.rows; r++ {
			s.grid[r] = blankRow(s.cols)
		}
	case 1:
		s.eraseLine(1)
		for r := 0; r < s.row; r++ {
			s.grid[r] = blankRow(s.cols)
		}
	default:
		for r := range s.grid {
			s.grid[r] = blankRow(s.cols)
		}
	}
}

func rowText(row []rune) string {
	return strings.TrimRight(string(row), " ")
}

// String is the scrolled-off rows and the screen's rows down to the last one with text, the last max of them.
func (s *screenTail) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := append([]string(nil), s.scrolled...)
	last := -1
	for r := range s.grid {
		if rowText(s.grid[r]) != "" {
			last = r
		}
	}
	for r := 0; r <= last; r++ {
		lines = append(lines, rowText(s.grid[r]))
	}
	if len(lines) > s.max {
		lines = lines[len(lines)-s.max:]
	}
	return strings.Join(lines, "\n")
}
