// Usage: go run mkterm.go | gofmt > ../xterm.go
package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pborman/pty/ansi"
)

const (
	escape  = 033    // untyped escape code
	bell    = 007    // bell/alert
	sescape = "\033" // untyped string of just an escape
)

var (
	ctlseqsLines []string
	ctlseqsIndex int
)

func crackIP(line string) ([]string, bool) {
	if !strings.HasPrefix(line, ".IP ") {
		return nil, false
	}
	line = strings.Replace(line, `\\`, `\`, -1)
	parts := expand(line)
	if len(parts) < 2 {
		return nil, true
	}
	switch parts[1] {
	case "PM":
	case "APC":
	case "CSI":
	case "DCS":
	case "ESC":
	case "OSC":
	default:
		// return nil, false
	}
	var rparts []string
	for _, p := range parts[1:] {
		if p != " " {
			rparts = append(rparts, p)
		}
	}
	return rparts, true
}

func isParam(s string) bool {
	return len(s) == 2 && s[0] == 'P' && s[1] >= 'a' && s[1] <= 'z'
}
func toString(s string) string {
	if ns := code2string[s]; ns != "" {
		return ns
	}
	return s
}
func toStrings(ss []string) string {
	ns := make([]string, len(ss))
	for x, s := range ss {
		ns[x] = toString(s)
	}
	return strings.Join(ns, "")
}

// Return the index of the first parameter
func param1(ss []string) int {
	for x, s := range ss {
		if isParam(s) {
			return x
		}
	}
	return len(ss) // No parameters found
}

// Return the index of the word after the last parameter
func paramN(ss []string) int {
	for x := len(ss) - 1; x > 0; x-- {
		if isParam(ss[x]) {
			return x + 1
		}
	}
	return len(ss) // No parameters found
}

func main() {
	ctlseqsGet()
	// skipTo(".Sh", "C1 (8-Bit) Control Characters")
	var mode, cmode string
	var desc string
	var names []string
	n2c := map[string]string{}
	seen := map[string]bool{}
	fmt.Printf(`
// Package xterm provides additional escape sequences recognized by xterm
// that are not included in ansi.Table.  Normally the only direct reference
// to the xterm package is:
//
//	if err := xterm.Import; err != nil {
//		// This should not happen.
//		// err contains the list of duplicated entries
//	}
package xterm

import (
	"fmt"

	"github.com/pborman/pty/ansi"
)

const ESC = 033

// Import imports the xterm code tables into the ansi table.
func Import() error {
	dups := ansi.Import(Table)
	if len(dups) == 0 {
		return nil
	}
	return fmt.Errorf("duplicated codes: %%q", dups)
}

`)

Looking:
	for {
		line, bool := ctlseqsNext()
		if bool == false {
			break
		}
		for _, mode := range []string{
			"Sixel Graphics",
			"ReGIS Graphics",
			"Tektronix 4014 Mode",
			"VT52 Mode",
		} {
			if lineCheck(line, ".Sh", mode) {
				skipTo(".Ed", "")
				continue Looking
			}
		}
		if lineCheck(line, ".Sh", "") {
			mode = strings.TrimSpace(line[3:])
		}
		if !lineCheck(line, ".St", "") {
			continue
		}
		for {
			line, bool := ctlseqsNext()
			if bool == false {
				return
			}
			if lineCheck(line, ".Ss", "") {
				desc = strings.TrimSpace(line[3:])
				if desc == "Operating System Commands" {
					break
				}
				continue
			}
			if lineCheck(line, ".Ed", "") {
				desc = ""
				break
			}
			p, ok := crackIP(line)
			if !ok || len(p) == 0 {
				continue
			}
			cp := p
			switch cp[len(p)-1] {
			case "ST", "BEL", "+1":
				if len(cp) > 1 {
					cp = cp[:len(cp)-1]
				}
			}
			f := param1(cp)
			n := paramN(cp)
			var codes []string
			codes = append(codes, cp[:f]...)
			codes = append(codes, cp[n:]...)
			code := toStrings(codes)
			if code == " " {
				continue
			}
			if ansi.Table[ansi.Name(code)] != nil {
				continue
			}
			if seen[code] {
				continue
			}
			seen[code] = true
			params := cp[f:n]
			_ = params

			if mode != "" {
				if desc == "" {
					fmt.Printf(`

// Mode %s

`, mode)
				}
				cmode = mode
				mode = ""
			}
			if desc != "" {
				fmt.Printf(`
// Mode %s %s

`, cmode, expands(desc))
				desc = ""
			}
			name := handcodes[code]
			if name == "" {

				line, _ = ctlseqsNext()
				if line != "" && line[0] == '.' {
					line = ""
					ctlseqsIndex--
				}
				if x := strings.Index(line, "("); x >= 0 {
					line = line[x+1:]
					if x := strings.Index(line, ")"); x >= 0 {
						name = line[:x]
					}
				}
				name = expands(name)
				if x := strings.Index(name, ","); x > 0 {
					name = name[:x]
				}
				if x := strings.Index(name, " is 0x"); x > 0 {
					name = name[:x]
				}
			}
			names = append(names, name)
			fmt.Printf("var %s_ = ansi.Sequence{\n", name)
			fmt.Printf("	Name: %q,\n", name)
			fmt.Printf("	Code: %s\n", expandCode(code))
			fmt.Printf("}\n\n")
			n2c[name] = code
		}
	}
	fmt.Printf("const (\n")
	for _, name := range names {
		code := fmt.Sprintf("%q", n2c[name])
		code = strings.Replace(code, `\x1b`, `\033`, 1)
		fmt.Printf("	%s = ansi.Name(%s)\n", name, code)
	}
	fmt.Printf(")\n")
	fmt.Printf("var Table = map[ansi.Name]*ansi.Sequence{\n")
	for _, name := range names {
		fmt.Printf("	%s: &%s_,\n", name, name)
	}
	fmt.Printf("}\n")
}

func expandCode(code string) string {
	parts := make([]string, len(code))
	for i, c := range code {
		if c == escape {
			parts[i] = "ESC"
			continue
		}
		s := fmt.Sprintf("%q", c)
		s = s[1 : len(s)-1]
		parts[i] = "'" + s + "'"
	}
	return fmt.Sprintf("[]byte{%s},", strings.Join(parts, ", "))
}

func ctlseqsGet() {
	data, err := ioutil.ReadFile("ctlseqs.ms")
	if err != nil {
		panic(err)
	}
	ctlseqsLines = strings.Split(string(data), "\n")
}

func ctlseqsNext() (string, bool) {
	if ctlseqsIndex < len(ctlseqsLines) {
		line := ctlseqsLines[ctlseqsIndex]
		ctlseqsIndex++
		return line, true
	}
	return "", false
}

func lineCheck(line, prefix, contents string) bool {
	return strings.HasPrefix(line, prefix) &&
		strings.Contains(line, contents)

}

func skipTo(prefix, contents string) bool {
	for {
		line, ok := ctlseqsNext()
		if !ok {
			return false
		}
		if lineCheck(line, prefix, contents) {
			return true
		}
	}
}

var escapelist = []struct {
	from, to string
}{
	// This are font shift codes, we don't care about them
	{`\f1`, ""},
	{`\fB`, ""},
	{`\fI`, ""},
	{`\fP`, ""},
	{`\fR`, ""},

	{`\*(''`, `"`},  // close quote
	{"\\*(``", `"`}, // open quote
	{`\*(((`, "("},
	{`\*(AP`, "APC"},
	{`\*(Be`, "BEL"},
	{`\*(Bs`, "BS"},
	{`\*(Ca`, "CAN"},
	{`\*(Cb`, "Cb"},
	{`\*(Cc`, "+1"},
	{`\*(Cr`, "CR"},
	{`\*(Cs`, "CSI"},
	{`\*(Cx`, "Cx"},
	{`\*(Cy`, "Cy"},
	{`\*(Dc`, "DCS"},
	{`\*(ET`, "ignore"},
	{`\*(Eb`, "ETB"},
	{`\*(Eg`, "EPA"},
	{`\*(En`, "ENQ"},
	{`\*(Es`, "ESC"},
	{`\*(Et`, "ETX"},
	{`\*(Ff`, "FF"},
	{`\*(Fs`, "FS"},
	{`\*(Gs`, "GS"},
	{`\*(Ht`, "HTS"},
	{`\*(Ic`, "c"},
	{`\*(Id`, "IND"},
	{`\*(Ir`, "r"},
	{`\*(Ix`, "x"},
	{`\*(Iy`, "y"},
	{`\*(LF`, "ignore"},
	{`\*(Lf`, "LF"},
	{`\*(Nl`, "NEL"},
	{`\*(Os`, "OSC"},
	{`\*(PM`, "PM"},
	{`\*(Pa`, "Pa"},
	{`\*(Pb`, "Pb"},
	{`\*(Pc`, "Pc"},
	{`\*(Pd`, "Pd"},
	{`\*(Pe`, "Pe"},
	{`\*(Pg`, "Pg"},
	{`\*(Ph`, "Ph"},
	{`\*(Pi`, "Pi"},
	{`\*(Pl`, "Pl"},
	{`\*(Pm`, "Pm"},
	{`\*(Pn`, "Pn"},
	{`\*(Pp`, "Pp"},
	{`\*(Pr`, "Pr"},
	{`\*(Ps`, "Ps"},
	{`\*(Pt`, "Pt"},
	{`\*(Pu`, "Pu"},
	{`\*(Pv`, "Pv"},
	{`\*(RF`, "ignore"},
	{`\*(RI`, "RI"},
	{`\*(Rs`, "RS"},
	{`\*(S2`, "SS2"},
	{`\*(S3`, "SS3"},
	{`\*(SS`, "SOS"},
	{`\*(ST`, "ST"},
	{`\*(Sc`, "ignore"},
	{`\*(Sg`, "SPA"},
	{`\*(Si`, "SI"},
	{`\*(So`, "SO"},
	{`\*(Sp`, "SP"},
	{`\*(Su`, "SUB"},
	{`\*(Ta`, "TAB"},
	{`\*(Us`, "US"},
	{`\*(Vt`, "VT"},
	{`\*(XT`, "XTerm"},
	{`\*(XX`, "X"},
	{`\*([[`, "["},
	{`\*(]]`, "]"},
	{`\*(bS`, `\`},
	{`\*(c"`, `"`},
	{`\*(cB`, "B"},
	{`\*(cs`, "s"},
	{`\*(c~`, "~"},
	{`\*(qu`, "`"},
	{`\*(xt`, "xterm"},

	{`\*XX`, `X`},
	{`\*[[`, `[`},
	{`\*]]`, `]`},
	{`\*bS`, `\`},
	{`\*cB`, `B`},
	{`\*cs`, `s`},
	{`\*qu`, `'`},

	{"\\*`", "`"},
	{`\*!`, `!`},
	{`\*#`, `#`},
	{`\*$`, `$`},
	{`\*%`, `%`},
	{`\*&`, `&`},
	{`\*(`, `(`},
	{`\*)`, `)`},
	{`\**`, `*`},
	{`\*+`, `+`},
	{`\*,`, `,`},
	{`\*-`, `-`},
	{`\*.`, `.`},
	{`\*/`, `/`},
	{`\*0`, `0`},
	{`\*1`, `1`},
	{`\*2`, `2`},
	{`\*3`, `3`},
	{`\*4`, `4`},
	{`\*5`, `5`},
	{`\*6`, `6`},
	{`\*7`, `7`},
	{`\*8`, `8`},
	{`\*9`, `9`},
	{`\*:`, `:`},
	{`\*;`, `;`},
	{`\*<`, `<`},
	{`\*=`, `=`},
	{`\*>`, `>`},
	{`\*?`, `?`},
	{`\*@`, `@`},
	{`\*A`, `A`},
	{`\*C`, `C`},
	{`\*D`, `D`},
	{`\*E`, `E`},
	{`\*F`, `F`},
	{`\*G`, `G`},
	{`\*H`, `H`},
	{`\*I`, `I`},
	{`\*J`, `J`},
	{`\*K`, `K`},
	{`\*L`, `L`},
	{`\*M`, `M`},
	{`\*N`, `N`},
	{`\*O`, `O`},
	{`\*P`, `P`},
	{`\*Q`, `Q`},
	{`\*R`, `R`},
	{`\*S`, `S`},
	{`\*T`, `T`},
	{`\*V`, `V`},
	{`\*W`, `W`},
	{`\*Y`, `Y`},
	{`\*Z`, `Z`},
	{`\*]`, `]`},
	{`\*^`, `^`},
	{`\*_`, `_`},
	{`\*a`, `a`},
	{`\*b`, `b`},
	{`\*c`, `c`},
	{`\*d`, `d`},
	{`\*e`, `e`},
	{`\*f`, `f`},
	{`\*g`, `g`},
	{`\*h`, `h`},
	{`\*i`, `i`},
	{`\*j`, `j`},
	{`\*k`, `k`},
	{`\*l`, `l`},
	{`\*m`, `m`},
	{`\*n`, `n`},
	{`\*o`, `o`},
	{`\*p`, `p`},
	{`\*q`, `q`},
	{`\*r`, `r`},
	{`\*s`, " "},
	{`\*t`, `t`},
	{`\*u`, `u`},
	{`\*v`, `v`},
	{`\*w`, `w`},
	{`\*x`, `x`},
	{`\*y`, `y`},
	{`\*z`, `z`},
	{`\*{`, `{`},
	{`\*|`, `|`},
	{`\*}`, `}`},
	{`\*~`, `~`},
}

var code2string = map[string]string{
	"APC": sescape + "_",
	"BEL": string(bell),
	"BS":  string('H' & 037),
	"CAN": string('X' & 037),
	"CR":  "\r",
	"CSI": sescape + "[",
	"DCS": sescape + "P",
	"ENQ": string('E' & 037),
	"ESC": sescape,
	"ETB": string('W' & 037),
	"ETX": string('C' & 037),
	"FF":  string('L' & 037),
	"FS":  string('\\' & 037),
	"LF":  "\n",
	"OSC": sescape + "]",
	"PM":  sescape + "^",
	"SI":  string('O' & 037),
	"SO":  string('N' & 037),
	"SP":  " ",
	"ST":  sescape + `\`,
	"SUB": string('Z' & 037),
	"TAB": "\t",
	"VT":  string('K' & 037),
}

var handcodes = map[string]string{
	"\x1b L":   "SANC1",
	"\x1b M":   "SANC2",
	"\x1b N":   "SANC3",
	"\x1b#3":   "DECDHLT",
	"\x1b#4":   "DECDHLB",
	"\x1b%@":   "SDCS",
	"\x1b%G":   "SUTF8",
	"\x1b(":    "DESG0CS",
	"\x1b)":    "DESG1CS",
	"\x1b*":    "DESG2CS",
	"\x1b+":    "DESG3CS",
	"\x1b-":    "DESG1CSVT300",
	"\x1b.":    "DESG2CSVT300",
	"\x1b/":    "DESG3CSVT300",
	"\x1bl":    "HPMEMLOCK",
	"\x1bm":    "HPMEMUNLOCK",
	"\x1bP+p":  "SETTI",
	"\x1bP+q":  "REQTI",
	"\x1b[?S":  "SUSIXEL",
	"\x1b[>T":  "RESETTITLE",
	"\x1b[>c":  "SENDDA",
	"\x1b[?i":  "DECMC",
	"\x1b[>m":  "DECSETMOD",
	"\x1b[>n":  "DECDISMOD",
	"\x1b[>p":  "SETPRINT",
	"\x1b[$p":  "DECRQMANSI",
	"\x1b[?$p": "DECRQMPRIVATE",
	"\x1b[r":   "DECSTBM",
	"\x1b[?r":  "DECREST",
	"\x1b[?s":  "DECSAVMOD",
	"\x1b[t":   "WINMOD",
	"\x1b[>t":  "SETTITLE",
	"\x1b[`}":  "DECIC",
	"\x1b[`~":  "DECDC",
}

var parameters = map[string]bool{
	"Pa": true,
	"Pb": true,
	"Pc": true,
	"Pg": true,
	"Ph": true,
	"Pi": true,
	"Pl": true,
	"Pm": true,
	"Pp": true,
	"Pr": true,
	"Ps": true,
	"Pt": true,
	"Pu": true,
	"Pv": true,
}

func expands(line string) string {
	return strings.Join(expand(line), "")
}

// Expand expands line into it's pieces.
func expand(line string) []string {
	var parts []string
Working:
	for len(line) > 0 {
		x := strings.Index(line, `\`)
		if x < 0 {
			parts = append(parts, line)
			break
		}
		if x > 0 {
			parts = append(parts, line[:x])
			line = line[x:]
		}
		for _, p := range escapelist {
			if strings.HasPrefix(line, p.from) {
				if p.to != "" {
					parts = append(parts, p.to)
				}
				line = line[len(p.from):]
				continue Working
			}
		}
		parts = append(parts, line[:1])
		line = line[1:]
	}
	return parts
}
