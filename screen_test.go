package main

import "testing"

// These are 1, 2 and 3 glyph wide and half width character strings.
const (
	wA = "日"
	wB = "本"
	wC = "語"
	w1 = wA
	w2 = wA + wB
	w3 = wA + wB + wC
	hA = "ｱ"
	hB = "ｲ"
	hC = "ｳ"
	h1 = hA
	h2 = hA + hB
	h3 = hA + hB + hC

	nw1 = len(w1)
	nw2 = len(w2)
	nw3 = len(w3)
	nh1 = len(h1)
	nh2 = len(h2)
	nh3 = len(h3)
)

func TestLineSetWidth(t *testing.T) {
	for _, tt := range []struct {
		line  string
		ncols int
	}{
		{"", 0},
		{"a", 1},
		{"abc", 3},
		{w1, 2},
		{w2, 4},
		{w3, 6},
		{h1, 1},
		{h2, 2},
		{h3, 3},
		{"a" + w1, 3},
		{w1 + "a", 3},
		{"a" + w1 + "b", 4},
		{w1 + "a" + w2, 7},
		{"abc" + h3 + w3, 12},
		{h1, 1},
		{h2, 2},
		{h3, 3},
		{"a" + h1 + "b" + h2, 5},
	} {
		l := &Line{bytes: []byte(tt.line)}
		l.setWidth()
		if l.ncols != tt.ncols {
			t.Errorf("%q: got width %d, want %d", tt.line, l.ncols, tt.ncols)
		}
	}
}
func TestNextWidth(t *testing.T) {
	for _, tt := range []struct {
		lcol int
		in   string
		n    int
		c    int
	}{
		{0, "", 0, 0},
		{0, "a", 1, 1},
		{0, "ab", 1, 1},
		{0, w2, 2, len(w1)},
		{0, h2, 1, len(h1)},
		{0, "\t", tabWidth - 0, 1},
		{1, "\t", tabWidth - 1, 1},
		{2, "\t", tabWidth - 2, 1},
		{tabWidth*3 + 2, "\t", tabWidth - 2, 1},
	} {
		n, c := nextWidth(tt.lcol, []byte(tt.in))
		if n != tt.n || c != tt.c {
			t.Errorf("nextWidth(%d, %q) got %d, %v, want %d, %v", tt.lcol, tt.in, n, c, tt.n, tt.c)
		}
	}
}

func TestLineMakeCols(t *testing.T) {
	for _, tt := range []struct {
		line string
		cols int
		out  string
		n    int
		b    bool
	}{
		{line: "", cols: 0},
		{line: "\t", cols: 0, out: "\t"},
		{line: "\t", cols: 1, out: "\t", b: true},
		{line: "\t", cols: 2, out: "\t", b: true},
		{line: "\t", cols: 3, out: "\t", b: true},
		{line: "\t", cols: tabWidth - 1, out: "\t", b: true},
		{line: "\t", cols: tabWidth, out: "\t", n: 1},
		{line: "\t", cols: tabWidth + 1, out: "\t ", n: 2},

		{line: "abc\t", cols: 0, out: "abc\t"},
		{line: "abc\t", cols: 3, out: "abc\t", n: 3},
		{line: "abc\t", cols: 4, out: "abc\t", n: 3, b: true},
		{line: "abc\t", cols: tabWidth - 1, out: "abc\t", n: 3, b: true},
		{line: "abc\t", cols: tabWidth, out: "abc\t", n: 4},
		{line: "abc\t", cols: tabWidth + 1, out: "abc\t ", n: 5},

		{line: "", cols: 1, out: " ", n: 1},
		{line: "", cols: 2, out: "  ", n: 2},
		{line: "", cols: 3, out: "   ", n: 3},
		{line: "  ", cols: 1, out: "  ", n: 1},
		{line: "  ", cols: 3, out: "   ", n: 3},
		{line: " ", cols: 0, out: " "},
		{line: " ", cols: 1, out: " ", n: 1},
		{line: " ", cols: 2, out: "  ", n: 2},
		{line: w1, cols: 0, out: w1, n: 0},
		{line: w1, cols: 1, out: w1, n: 0, b: true},
		{line: w1, cols: 2, out: w1, n: nw1},
		{line: w1, cols: 3, out: w1 + " ", n: nw1 + 1},
		{line: "a" + w1, cols: 2, out: "a" + w1, n: 1, b: true},
		{line: "ab" + w1, cols: 2, out: "ab" + w1, n: 2},
		{line: "ab" + w1, cols: 3, out: "ab" + w1, n: 2, b: true},
		{line: "ab" + w1, cols: 4, out: "ab" + w1, n: nw1 + 2},
		{line: "abc" + w1, cols: 3, out: "abc" + w1, n: 3},
		{line: "abc" + w1, cols: 4, out: "abc" + w1, n: 3, b: true},
	} {
		l := &Line{bytes: []byte(tt.line)}
		l.setWidth()
		n, b := l.makeCols(tt.cols)
		out := string(l.bytes)
		if out != tt.out || n != tt.n || b != tt.b {
			t.Errorf("makeCols(%q, %d)\ngot  %q, %d, %v\nwant %q, %d, %v", tt.line, tt.cols, out, n, b, tt.out, tt.n, tt.b)
		}
	}
}

func TestLineInsert(t *testing.T) {
	for _, tt := range []struct {
		line   string
		col    int
		insert string
		out    string
		ncols  int
	}{
		{line: "", col: 0, insert: "a", out: "a", ncols: 1},
		{line: "", col: 2, insert: "a", out: "  a", ncols: 3},
		{line: "", col: 3, insert: "a", out: "   a", ncols: 4},
		{line: " ", col: 0, insert: "a", out: "a ", ncols: 2},
		{line: " ", col: 1, insert: "a", out: " a", ncols: 2},
		{line: w1, col: 0, insert: "a", out: "a" + w1, ncols: 3},
		{line: w1, col: 1, insert: "a", out: "a" + w1, ncols: 3},
		{line: w1, col: 2, insert: "a", out: w1 + "a", ncols: 3},
		{line: w1, col: 3, insert: "a", out: w1 + " a", ncols: 4},
		{line: w2, col: 0, insert: "a", out: "a" + w2, ncols: 5},
		{line: w2, col: 1, insert: "a", out: "a" + w2, ncols: 5},
		{line: w2, col: 2, insert: "a", out: w1 + "a" + wB, ncols: 5},
		{line: hA + hB, col: 1, insert: "a", out: hA + "a" + hB, ncols: 3},
	} {
		l := &Line{bytes: []byte(tt.line)}
		l.setWidth()
		l.insert([]byte(tt.insert), tt.col)
		out := string(l.bytes)
		if out != tt.out || l.ncols != tt.ncols {
			t.Errorf("insert(%q, %q, %d) %q\ngot  %q, %d\nwant %q, %d", tt.line, tt.insert, tt.col, spaces, out, l.ncols, tt.out, tt.ncols)
		}
	}
}

func TestLineReplace(t *testing.T) {
	for _, tt := range []struct {
		line    string
		col     int
		replace string
		out     string
		ncols   int
	}{
		{line: "", col: 0, replace: "a", out: "a", ncols: 1},
		{line: "", col: 2, replace: "a", out: "  a", ncols: 3},
		{line: "", col: 3, replace: "a", out: "   a", ncols: 4},
		{line: " ", col: 0, replace: "a", out: "a", ncols: 1},
		{line: " ", col: 1, replace: "a", out: " a", ncols: 2},
		{line: w1, col: 0, replace: "a", out: "a", ncols: 1},
		{line: w1, col: 1, replace: "a", out: "a", ncols: 1},
		{line: w1, col: 2, replace: "a", out: w1 + "a", ncols: 3},
		{line: w1, col: 3, replace: "a", out: w1 + " a", ncols: 4},
		{line: w2, col: 0, replace: "a", out: "a" + wB, ncols: 3},
		{line: w2, col: 1, replace: "a", out: "a" + wB, ncols: 3},
		{line: w2, col: 2, replace: "a", out: w1 + "a", ncols: 3},
		{line: w2, col: 3, replace: "a", out: w1 + "a", ncols: 3},
		{line: wA + wB + wC, col: 2, replace: "a", out: wA + "a" + wC, ncols: 5},
		{line: wA + wB + wC, col: 2, replace: hB, out: wA + hB + wC, ncols: 5},
		{line: "abc", col: 1, replace: wA, out: "a" + wA + "c", ncols: 4},
		{line: "abc", col: 1, replace: hA, out: "a" + hA + "c", ncols: 3},
	} {
		l := &Line{bytes: []byte(tt.line)}
		l.setWidth()
		l.replace([]byte(tt.replace), tt.col)
		out := string(l.bytes)
		if out != tt.out || l.ncols != tt.ncols {
			t.Errorf("replace(%q, %q, %d) %q\ngot  %q, %d\nwant %q, %d", tt.line, tt.replace, tt.col, spaces, out, l.ncols, tt.out, tt.ncols)
		}
	}
}

func TestLineDelete(t *testing.T) {
	for _, tt := range []struct {
		line  string
		col   int
		out   string
		ncols int
	}{
		{line: ""},
		{line: "", col: 1},
		{line: "", col: 2},
		{line: "a", col: 0, out: ""},
		{line: "a", col: 1, out: "a", ncols: 1},
		{line: "abc", col: 1, out: "ac", ncols: 2},
		{line: wA + wB + wC, col: 0, out: wB + wC, ncols: 4},
		{line: wA + wB + wC, col: 1, out: wB + wC, ncols: 4},
		{line: wA + wB + wC, col: 2, out: wA + wC, ncols: 4},
		{line: wA + wB + wC, col: 3, out: wA + wC, ncols: 4},
		{line: wA + wB + wC, col: 4, out: wA + wB, ncols: 4},
		{line: wA + wB + wC, col: 5, out: wA + wB, ncols: 4},
		{line: wA + wB + wC, col: 6, out: wA + wB + wC, ncols: 6},
		{line: wA + wB + wC, col: 7, out: wA + wB + wC, ncols: 6},
		{line: wA + wB, col: 2, out: wA, ncols: 2},
		{line: wA + wB, col: 3, out: wA, ncols: 2},
		{line: "a" + wA + "b" + wB, col: 0, out: wA + "b" + wB, ncols: 5},
		{line: "a" + wA + "b" + wB, col: 1, out: "ab" + wB, ncols: 4},
		{line: "a" + wA + "b" + wB, col: 2, out: "ab" + wB, ncols: 4},
		{line: "a" + wA + "b" + wB, col: 3, out: "a" + wA + wB, ncols: 5},
		{line: "a" + wA + "b" + wB, col: 4, out: "a" + wA + "b", ncols: 4},
		{line: "a" + wA + "b" + wB, col: 5, out: "a" + wA + "b", ncols: 4},
	} {
		l := &Line{bytes: []byte(tt.line)}
		l.setWidth()
		l.delete(tt.col)
		out := string(l.bytes)
		if out != tt.out || l.ncols != tt.ncols {
			t.Errorf("delete(%q, %d) %q\ngot  %q, %d\nwant %q, %d", tt.line, tt.col, spaces, out, l.ncols, tt.out, tt.ncols)
		}
	}
}
