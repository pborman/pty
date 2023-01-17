package main

import (
	"io"

	"github.com/pborman/pty/ansi"
	"github.com/pborman/pty/log"
	"golang.org/x/text/width"
)

type Line struct {
	ncols int
	bytes []byte
}

var spaces = []byte("        ")

const tabWidth = 8

func getSpaces(n int) []byte {
	for len(spaces) < n {
		spaces = append(spaces, spaces...)
	}
	return spaces[:n]
}

// makeCols makes Line l be at least cols long by padding with spaces.
// makeCols returns the index of the byte where the next column starts.
// makeCols returns the index of the start of the following column.
// makeCols returns true if column cols is in the middle of a wide
// character, in which case the index prior to the wide charcter is returned.
// For example, if the first glyph in a line is a wide character, then
// makeCols(1) will return (0, true).
//
// makeCols(0) always returns 0, false.
func (l *Line) makeCols(cols int) (int, bool) {
	if l.bytes == nil {
		l.bytes = make([]byte, 0, 80)
	}
	if cols == 0 {
		return 0, false
	}
	if cols == l.ncols {
		return len(l.bytes), false
	}
	// If we are requesting more than columns than we have,
	// just append spaces to get the the colum and return the
	// index of the last byte (it is the start of columns cols).
	if cols >= l.ncols {
		l.bytes = append(l.bytes, getSpaces(cols-l.ncols)...)
		l.ncols = cols
		return len(l.bytes), false
	}

	li := 0
	i := 0
	cc := 0
	for cc < cols {
		w, n := nextWidth(cc, l.bytes[i:])
		li = i
		cc += w
		i += n
	}
	if cols < cc {
		return li, true
	}
	return i, false
}

// TODO: Need to have an insert and replace that take a maximum number of
// columns. For example, if the maximum number of columns was 12 and we
// currently has "abcdefg" then inserting a tab before a or b should cause "fg"
// to overflow.  If the maximum is 4 and we currently have "abcd" then inserting
// a character before any of those letters will cause d to overlow.  Inserting a
// doublewidth character anyplace before c will cause cd to overflow.  Inserting
// a doublewidth character (WW) after c should cause both WW and d to overflow
// and the line would now only have "abc".  Replacing c with WW could likewise
// cause d to overflow while replacing d with WW will result in WW being
// overflow and the resulting line is again just "abc".

// insert buf at column position col in Line l.  Column numbers start at 0,
// so 0 means the start of the line and 1 means after the first column.
func (l *Line) insert(buf []byte, col int) {
	i, _ := l.makeCols(col)
	switch i {
	case 0:
		l.bytes = append(append([]byte{}, buf...), l.bytes...)
	case len(l.bytes):
		l.bytes = append(l.bytes, buf...)
	default:
		buf = append([]byte{}, buf...)
		l.bytes = append(l.bytes[:i], append(buf, l.bytes[i:]...)...)
	}
	w, _ := nextWidth(col, buf)
	l.ncols += w
}

// delete deletes the glyph at column col.
func (l *Line) delete(col int) {
	if col >= l.ncols {
		return
	}
	i := 0
	c := 0
	for {
		w, n := nextWidth(c, l.bytes[i:])
		c += w
		if c > col {
			l.bytes = append(l.bytes[:i], l.bytes[i+n:]...)
			l.setWidth()
			return
		}
		i += n
	}
}

// replace column col with buf.  If col is the second column of a wide
// character then col-1 and col are both replaced.
func (l *Line) replace(buf []byte, col int) {
	i, _ := l.makeCols(col)
	_, w := nextWidth(col, l.bytes[i:])
	if w == 0 {
		l.bytes = append(l.bytes, buf...)
	} else {
		buf = append([]byte{}, buf...)
		l.bytes = append(l.bytes[:i], append(buf, l.bytes[i+w:]...)...)
	}
	l.setWidth()
}

func (l *Line) setWidth() {
	l.ncols = 0
	for i := 0; i < len(l.bytes); {
		w, n := nextWidth(l.ncols, l.bytes[i:])
		l.ncols += w
		i += n
	}
}

// runeWidth returns the number of columns a particular rune uses.
// EastAsianAmbiguous is considered wide, even though it might not be.
func runeWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianAmbiguous:
		return 2
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

func nextWidth(lcol int, buf []byte) (cols int, consumed int) {
	if len(buf) == 0 {
		return 0, 0
	}

	if buf[0] == '\t' {
		return tabWidth - (lcol % tabWidth), 1
	}

	p, consumed := width.Lookup(buf)

	switch p.Kind() {
	case width.EastAsianAmbiguous:
		return 2, consumed
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2, consumed
	default:
		return 1, consumed
	}
}

type Screen struct {
	pr      *io.PipeReader
	pw      *io.PipeWriter
	history []Line
	screen  []Line
	cr, cc  int // current cursor position
}

func NewScreen() *Screen {
	var s Screen
	s.pr, s.pw = io.Pipe()
	go s.run()
	return &s
}

func (s *Screen) Winch(rows, cols int) {
	if s == nil {
		return
	}
}

func (s *Screen) Write(buf []byte) (int, error) {
	return s.pw.Write(buf)
}

func (s *Screen) run() {
	defer func() {
		log.Errorf("Screen is done")
		if p := recover(); p != nil {
			log.Errorf("Panic: %v", p)
			log.DumpStack()
			panic(p)
		}

	}()
	d := ansi.NewReader(s.pr)
	unknown := map[ansi.Name]bool{}
	for {
		seq, err := d.Next()
		if err != nil {
			return
		}
		// We either have an escape sequence or raw bytes of data.
		switch seq.Code {
		case "":
			// seq.Text is plain next
		default:
			if !unknown[seq.Code] {
				unknown[seq.Code] = true
			}
		}
	}
}
