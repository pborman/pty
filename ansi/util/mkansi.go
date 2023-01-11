// go run mkansi.go | gofmt > ../ansi.go
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// A Code contains data extracted from the ECMA-48 standard.
type Code struct {
	Name     string // Name of the sequence (CUU)
	Desc     string // Long name of the sequence  (Cursor Up)
	Rep      string // quoted string representation ("\033[A")
	Notation int // What sort of notation is used ("Pn1;Pn2")
	Seq      []int // Representation converted to bytes
	Def      []int // Default values
	Text     string // Descriptive text
}

// Constants used for sequences and notations.
const (
	CSI = 0x100 + iota
	ESC
	Pn
	Pn1
	Ps
	Ps1
	Psc
	Fs
	C0
	C1
)

// Map of symbols found to our code.
var spec = map[string]int{
	"CSI":     CSI, // Seq only
	"ESC":     ESC, // Seq only
	"Pn":      Pn,
	"Pn1;Pn2": Pn1,
	"Ps":      Ps,
	"Ps...":   Psc,
	"Ps1;Ps2": Ps1,
	"Fs":      Fs, // Notation only
	"C0":      C0, // Notation only
	"C1":      C1, // Notation only
}

// Map of codes to symbols
var ceps = map[int]string{}

func init() {
	for k, v := range spec {
		ceps[v] = k
	}
}

// crep converts a representation into a slice of integers.  For example:
// "CSI Pn 06/05" would become []int{CSI, Pn, 0x65}.
func crep(in string) (a []int) {
	words := strings.Fields(in)
	for _, w := range words {
		c := spec[w]
		if c == 0 {
			if len(w) != 5 || w[0] != '0' || w[2] != '/' {
				panic(in)
			}
			if w[1] < '0' || w[1] > '9' {
				panic(in)
			}
			if w[3] < '0' || w[3] > '9' {
				panic(in)
			}
			if w[4] < '0' || w[4] > '9' {
				panic(in)
			}
			c = int(w[1]-'0')*16 + int(w[3]-'0')*10 + int(w[4]-'0')
		}
		a = append(a, c)
	}
	return a
}

// cdef convers a list of default values into a slice of integers.
// For example: "Ps1 = 7; Ps2 = 11" would become []int{7, 11}
func cdef(in string) (defs []int) {
	for _, d := range strings.Split(in, ";") {
		w := strings.Split(d, "=")
		if len(w) != 2 {
			panic(in)
		}
		v, err := strconv.Atoi(strings.TrimSpace(w[1]))
		if err != nil {
			panic(in)
		}
		defs = append(defs, v)
	}
	return defs
}

// title takes the uppercase string in and returns a mixed case string
// suitable for display.
func title(in string) string {
	in = strings.Title(strings.ToLower(strings.TrimSpace(in)))
	for _, p := range []string{"And ", "In ", "Of ", "On ", "Or ", "To "} {
		in = strings.Replace(in, p, strings.ToLower(p), -1)
	}
	for _, p := range []string{"Fs ", "Gs ", "Rs ", "Us "} {
		in = strings.Replace(in, p, strings.ToUpper(p), -1)
	}
	return in

}

var L1 [256]*Code
var Other []*Code

// crack converts in into a Code, inserting it either into the L1 table
// or appending it to the Other table.
func crack(in string) {
	var c Code

	in = in[strings.Index(in, " ")+1:] // strip off leading number
	a := strings.SplitN(in, "-", 2)
	if len(a) != 2 {
		return
	}
	c.Name = strings.TrimSpace(a[0])
	in = a[1]
	a = strings.SplitN(in, "\n", 2)
	if len(a) != 2 {
		return
	}
	c.Desc = title(a[0])
	in = a[1]

	const N = "Notation: ("
	const R = "Representation: "

	x := strings.Index(in, N)
	if x < 0 {
		fmt.Fprintf(os.Stderr, "%s: no notation\n", c.Name)
		return
	}
	in = in[x+len(N):]
	x = strings.Index(in, ")")
	if x < 0 {
		fmt.Fprintf(os.Stderr, "%s: no notation )\n", c.Name)
		return
	}
	c.Notation = spec[in[:x]]
	if c.Notation == 0 {
		fmt.Fprintf(os.Stderr, "%s: bad notation %q\n", c.Name, in[:x])
		return
	}
	in = in[x:]

	x = strings.Index(in, R)
	if x < 0 {
		fmt.Fprintf(os.Stderr, "%s: no representation\n", c.Name)
		return
	}
	in = in[x+len(R):]
	x = strings.Index(in, "\n")
	if x < 0 {
		fmt.Fprintf(os.Stderr, "%s: no representation newline\n", c.Name)
		return
	}

	rep := in[:x]
	in = in[x+1:]

	const OE = " or ESC "
	alt := false
	if x := strings.Index(rep, OE); x >= 0 {
		rep = rep[x+len(OE)-4:]
		alt = true
	}
	if alt != (c.Notation == C1) {
		fmt.Fprintf(os.Stderr, "%s: bad notation\n", c.Name)
		return
	}

	c.Seq = crep(rep)
	switch c.Notation {
	case C0:
		if len(c.Seq) != 1 {
			fmt.Fprintf(os.Stderr, "%s: bad C0\n", c.Name)
			return
		}
		// 0x00 - 0x1f
		L1[c.Seq[0]] = &c
	case C1, Fs:
		// 0x40 - 0x5f - C1
		// 0x60 - 0x7f - Fs
		if len(c.Seq) != 2 || c.Seq[0] != ESC {
			fmt.Fprintf(os.Stderr, "%s: bad C1\n", c.Name)
			return
		}
		Other = append(Other, &c)
	case Pn, Pn1, Ps, Ps1, Psc:
		if c.Seq[0] != CSI {
			fmt.Fprintf(os.Stderr, "%s: bad\n", c.Name)
			return
		}
		Other = append(Other, &c)
	case ESC:
		Other = append(Other, &c)
	default:
		fmt.Fprintf(os.Stderr, "%s: bad notation: %q\n", c.Name, ceps[c.Notation])
	}

	const NPD = "No parameter default value"
	if strings.HasPrefix(in, NPD+".\n") {
		in = in[len(NPD)+2:]
	}

	ndf1 := false
	if strings.HasPrefix(in, NPD) {
		ndf1 = true
		a := strings.SplitN(in, "\n", 2)
		in = a[1]
	}
	if strings.HasPrefix(in, "Parameter default value") {
		a := strings.SplitN(in, "\n", 2)
		if len(a) != 2 {
			return
		}
		x := strings.Index(a[0], ":")
		if x < 0 {
			return
		}
		c.Def = cdef(a[0][x+1:])
		if ndf1 {
			c.Def = append([]int{-1}, c.Def...)
		}
		in = a[1]
	}

	c.Text = in
}

func main() {
	for _, line := range codes {
		crack(line)
	}

	fmt.Println(`// Package ansi provides ansi escape sequence processing as defined by the
// ECMA-48 standard "Control Functions for Coded Character Sets - Fifth Edition"
//
// From the standard:
//
//   Free printed copies of this standard can be ordered from:
//   ECMA
//   114 Rue du Rhône CH-1204 Geneva Switzerland
//   Fax: +41 22 849.60.01 Email: documents@ecma.ch
//   Files of this Standard can be freely downloaded from the ECMA web site
//   (www.ecma.ch). This site gives full information on ECMA, ECMA activities,
//   ECMA Standards and Technical Reports.
//
// Portions of the standared are included in the documentation for this package.
// The standard bears no copyright.
//
// Each escape sequence is represented by a string constant and a Sequence
// structure.  For the escape sequence named SEQ, SEQ is the string constant and
// SEQ_ is the related Sequence structure.  A mapping from a sequence string to
// its Sequence structure is provided in Table.
//
// Some escape sequences may contain parameters, for example "\033[4A".  This
// sequence contains the parameter "4".  The name of the sequences is "\033[A"
// (the parameter is missing).  The sequence "\033[1;2 c" is named "\033[ c" and
// has the parameters "1", and "2".
//
// The C1 control set has both a two byte and a single byte representation.  The
// two byte representation is an Escape followed by a byte in the range of 0x40
// to 0x5f.  They may also be specified by a single byte in the range of 0x80 -
// 0x9f.  This ansi package always names the C1 control set in the two byte
// form.
package ansi

`)

	fmt.Println(`
// A Sequence specifies an ANSI (ECMA-48) Escape Sequence.
// If the default value of a parameter is -1 then that parameter must
// be specified (but following parameters may be defaulted).
type Sequence struct {
	Name     string   // Short name of the sequence
	Desc     string   // Description of the sequence
	Notation string   // Notation the sequence uses
	Type     string   // Prefix type: ESC, CSI or ""
	NParam   int      // Number of parameters (-1 implies any number)
	MinParam int      // Minium number of parameters that must be present
	Defaults []string // Default values of parameters (if any)
	Code     []byte   // Code bytes
}`)

	fmt.Println(`
// ANSI (ECMA-48) Sequences.
// These sequences do not include parameters or string termination sequences.
const (`)
	for i, c := range L1 {
		if c != nil {
			c.Rep = fmt.Sprintf(`"\%03o"`, i)
			fmt.Printf(`	%s = %s // %s`+"\n", c.Name, c.Rep, c.Desc)
		}
	}
	for _, c := range Other {
		c.Rep = "\033"
		switch c.Seq[0] {
		case ESC:
		case CSI:
			c.Rep += "["
		default:
			panic("unkonwn seq: " + c.Name)
		}
		i := len(c.Seq)
		for ; i > 0 && c.Seq[i-1] < 0x100; i-- { }
		for _, r := range c.Seq[i:] {
			c.Rep += string(r)
		}
		c.Rep = strings.Replace(fmt.Sprintf("%q", c.Rep), `\x1b`, `\033`, -1)
		fmt.Printf("\t%s = %s // %s\n", c.Name, c.Rep, c.Desc)
	}
	fmt.Println(")")

	for _, c := range append(L1[:], Other...) {
		if c == nil {
			continue
		}
		n := 0
		m := 0
		switch c.Notation {
		case Ps, Pn:
			n = 1
			m = 1 - len(c.Def)
		case Ps1, Pn1:
			n = 2
			m = 2 - len(c.Def)
		case Psc:
			n = -1
		}
		if len(c.Def) > 0 && c.Def[0] == -1 {
			m++
		}

		fmt.Println()
		for x, line := range strings.Split(c.Text, "\n") {
			if x > 0 {
				fmt.Println("//")
			}
			for _, t := range breakup(line) {
				fmt.Println("//", t)
			}
		}
		fmt.Printf(`var %s_ = Sequence {
	Name: %q,
	Desc: %q,
`, c.Name, c.Name, c.Desc)
		if c.Notation == C0 {
			fmt.Printf("\tCode: []byte(%s),\n", c.Name)
			fmt.Printf("}\n")
			continue
		}
		switch c.Seq[0] {
		case CSI:
			fmt.Printf("\tType: CSI,\n")
			fmt.Printf("\tNotation: %q,\n", ceps[c.Notation])
		case ESC:
			fmt.Printf("\tType: ESC,\n")
		}
		if n != 0 {
			fmt.Printf("\tNParam: %d,\n", n)
		}
		if m > 0 {
			fmt.Printf("\tMinParam: %d,\n", m)
		}

		if len(c.Def) > 0 {
			fmt.Printf("\tDefaults: []string{")
			for _, d := range c.Def {
				if d >= 0 {
					fmt.Printf(`"%d", `, d)
				} else {
					fmt.Printf(`"", `)
				}
			}
			fmt.Println("},")
		}
		e := ""
		if len(c.Seq) > 1 {
			switch {
			case c.Seq[len(c.Seq)-2] < 0x100:
				e = "'" + string(c.Seq[len(c.Seq)-2]) + "', "
			}
		}
		b := string(c.Seq[len(c.Seq)-1])
		if b == `\` {
			b = `\\`
		}
		fmt.Printf("\tCode: []byte{%s'%s'},\n", e, b)
		fmt.Printf("}\n")
	}

	fmt.Println(`
// Table maps escape sequences to the corresponding Sequence.
// The sequence does not include parameters or string termination sequences.
var Table = map[string]*Sequence {`)
	for _, c := range append(L1[:], Other...) {
		if c == nil {
			continue
		}
		fmt.Printf("\t%s: &%s_,\n", c.Name, c.Name)
		if c.Notation == C1 {
		fmt.Printf("\t\"\\%03o\": &%s_,\n", c.Seq[len(c.Seq)-1] + 0x40, c.Name)
		}
	}
	fmt.Printf("}\n")
}

func breakup(in string) (out []string) {
	for len(in) > 77 {
		x := strings.LastIndex(in[:77], " ")
		out = append(out, in[:x])
		in = in[x+1:]
	}
	return append(out, in)
}

// This is section 8.3 of the EMCA-48 standard.
var codes = strings.Split(`
8.3.1 ACK - ACKNOWLEDGE
Notation: (C0) Representation: 00/06
ACK is transmitted by a receiver as an affirmative response to the sender.
The use of ACK is defined in ISO 1745.
8.3.2 APC - APPLICATION PROGRAM COMMAND
Notation: (C1)
Representation: 09/15 or ESC 05/15
APC is used as the opening delimiter of a control string for application program use. The command string following may consist of bit combinations in the range 00/08 to 00/13 and 02/00 to 07/14. The control string is closed by the terminating delimiter STRING TERMINATOR (ST). The interpretation of the command string depends on the relevant application program.
8.3.3 BEL - BELL
Notation: (C0) Representation: 00/07
BEL is used when there is a need to call for attention; it may control alarm or attention devices.
8.3.4 BPH - BREAK PERMITTED HERE
Notation: (C1)
Representation: 08/02 or ESC 04/02
BPH is used to indicate a point where a line break may occur when text is formatted. BPH may occur between two graphic characters, either or both of which may be SPACE.
8.3.5 BS - BACKSPACE
Notation: (C0) Representation: 00/08
BS causes the active data position to be moved one character position in the data component in the direction opposite to that of the implicit movement.
The direction of the implicit movement depends on the parameter value of SELECT IMPLICIT MOVEMENT DIRECTION (SIMD).
8.3.6 CAN - CANCEL
Notation: (C0) Representation: 01/08
CAN is used to indicate that the data preceding it in the data stream is in error. As a result, this data shall be ignored. The specific meaning of this control function shall be defined for each application and/or between sender and recipient.
8.3.7 CBT - CURSOR BACKWARD TABULATION
Notation: (Pn) Representation: CSI Pn 05/10
Parameter default value: Pn = 1
CBT causes the active presentation position to be moved to the character position corresponding to the n-th preceding character tabulation stop in the presentation component, according to the character path, where n equals the value of Pn.
8.3.8 CCH - CANCEL CHARACTER
Notation: (C1)
Representation: 09/04 or ESC 05/04
CCH is used to indicate that both the preceding graphic character in the data stream, (represented by one or more bit combinations) including SPACE, and the control function CCH itself are to be ignored for further interpretation of the data stream.
If the character preceding CCH in the data stream is a control function (represented by one or more bit combinations), the effect of CCH is not defined by this Standard.
8.3.9 CHA - CURSOR CHARACTER ABSOLUTE
Notation: (Pn) Representation: CSI Pn 04/07
Parameter default value: Pn = 1
CHA causes the active presentation position to be moved to character position n in the active line in the presentation component, where n equals the value of Pn.
8.3.10 CHT - CURSOR FORWARD TABULATION
Notation: (Pn) Representation: CSI Pn 04/09
Parameter default value: Pn = 1
CHT causes the active presentation position to be moved to the character position corresponding to the n-th following character tabulation stop in the presentation component, according to the character path, where n equals the value of Pn.
8.3.11 CMD - CODING METHOD DELIMITER
Notation: (Fs) Representation: ESC 06/04
CMD is used as the delimiter of a string of data coded according to Standard ECMA-35 and to switch to a general level of control.
The use of CMD is not mandatory if the higher level protocol defines means of delimiting the string, for instance, by specifying the length of the string.
8.3.12 CNL - CURSOR NEXT LINE
Notation: (Pn) Representation: CSI Pn 04/05
Parameter default value: Pn = 1
CNL causes the active presentation position to be moved to the first character position of the n-th following line in the presentation component, where n equals the value of Pn.
8.3.13 CPL - CURSOR PRECEDING LINE
Notation: (Pn) Representation: CSI Pn 04/06
Parameter default value: Pn = 1
CPL causes the active presentation position to be moved to the first character position of the n-th preceding line in the presentation component, where n equals the value of Pn.
8.3.14 CPR - ACTIVE POSITION REPORT
Notation: (Pn1;Pn2) Representation: CSI Pn1;Pn2 05/02
Parameter default values: Pn1 = 1; Pn2 = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, CPR is used to report the active presentation position of the sending device as residing in the presentation component at the n-th line position according to the line progression and at the m-th character position according to the character path, where n equals the value of Pn1 and m equals the value of Pn2.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, CPR is used to report the active data position of the sending device as residing in the data component at the n-th line position according to the line progression and at the m-th character position according to the character progression, where n equals the value of Pn1 and m equals the value of Pn2.
CPR may be solicited by a DEVICE STATUS REPORT (DSR) or be sent unsolicited.
8.3.15 CR - CARRIAGE RETURN
Notation: (C0) Representation: 00/13
The effect of CR depends on the setting of the DEVICE COMPONENT SELECT MODE (DCSM) and on the parameter value of SELECT IMPLICIT MOVEMENT DIRECTION (SIMD).
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION and with the parameter value of SIMD equal to 0, CR causes the active presentation position to be moved to the line home position of the same line in the presentation component. The line home position is established by the parameter value of SET LINE HOME (SLH).
With a parameter value of SIMD equal to 1, CR causes the active presentation position to be moved to the line limit position of the same line in the presentation component. The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA and with a parameter value of SIMD equal to 0, CR causes the active data position to be moved to the line home position of the same line in the data component. The line home position is established by the parameter value of SET LINE HOME (SLH).
With a parameter value of SIMD equal to 1, CR causes the active data position to be moved to the line limit position of the same line in the data component. The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
8.3.16 CSI - CONTROL SEQUENCE INTRODUCER
Notation: (C1)
Representation: 09/11 or ESC 05/11
CSI is used as the first character of a control sequence, see 5.4.
8.3.17 CTC - CURSOR TABULATION CONTROL
Notation: (Ps...) Representation: CSI Ps... 05/07
Parameter default value: Ps = 0
CTC causes one or more tabulation stops to be set or cleared in the presentation component, depending on the parameter values:
0 a character tabulation stop is set at the active presentation position
1 a line tabulation stop is set at the active line (the line that contains the active presentation position)
2 the character tabulation stop at the active presentation position is cleared
3 the line tabulation stop at the active line is cleared
4 all character tabulation stops in the active line are cleared
5 all character tabulation stops are cleared
6 all line tabulation stops are cleared
In the case of parameter values 0, 2 or 4 the number of lines affected depends on the setting of the TABULATION STOP MODE (TSM).
8.3.18 CUB - CURSOR LEFT
Notation: (Pn) Representation: CSI Pn 04/04
Parameter default value: Pn = 1
CUB causes the active presentation position to be moved leftwards in the presentation component by n character positions if the character path is horizontal, or by n line positions if the character path is vertical, where n equals the value of Pn.
8.3.19 CUD - CURSOR DOWN
Notation: (Pn) Representation: CSI Pn 04/02
Parameter default value: Pn = 1
CUD causes the active presentation position to be moved downwards in the presentation component by n line positions if the character path is horizontal, or by n character positions if the character path is vertical, where n equals the value of Pn.
8.3.20 CUF - CURSOR RIGHT
Notation: (Pn) Representation: CSI Pn 04/03
Parameter default value: Pn = 1
CUF causes the active presentation position to be moved rightwards in the presentation component by n character positions if the character path is horizontal, or by n line positions if the character path is vertical, where n equals the value of Pn.
8.3.21 CUP - CURSOR POSITION
Notation: (Pn1;Pn2) Representation: CSI Pn1;Pn2 04/08
Parameter default values: Pn1 = 1; Pn2 = 1
CUP causes the active presentation position to be moved in the presentation component to the n-th line position according to the line progression and to the m-th character position according to the character path, where n equals the value of Pn1 and m equals the value of Pn2.
8.3.22 CUU - CURSOR UP
Notation: (Pn) Representation: CSI Pn 04/01
Parameter default value: Pn = 1
CUU causes the active presentation position to be moved upwards in the presentation component by n line positions if the character path is horizontal, or by n character positions if the character path is vertical, where n equals the value of Pn.
8.3.23 CVT - CURSOR LINE TABULATION
Notation: (Pn) Representation: CSI Pn 05/09
Parameter default value: Pn = 1
CVT causes the active presentation position to be moved to the corresponding character position of the line corresponding to the n-th following line tabulation stop in the presentation component, where n equals the value of Pn.
8.3.24 DA - DEVICE ATTRIBUTES
Notation: (Ps) Representation: CSI Ps 06/03
Parameter default value: Ps = 0
With a parameter value not equal to 0, DA is used to identify the device which sends the DA. The parameter value is a device type identification code according to a register which is to be established. If the parameter value is 0, DA is used to request an identifying DA from a device.
8.3.25 DAQ - DEFINE AREA QUALIFICATION
Notation: (Ps...) Representation: CSI Ps... 06/15
Parameter default value: Ps = 0
DAQ is used to indicate that the active presentation position in the presentation component is the first character position of a qualified area. The last character position of the qualified area is the character position in the presentation component immediately preceding the first character position of the following qualified area.
The parameter value designates the type of qualified area:
0 unprotected and unguarded
1 protected and guarded
2 graphic character input
3 numeric input
4 alphabetic input
5 input aligned on the last character position of the qualified area
6 fill with ZEROs
7 set a character tabulation stop at the active presentation position (the first character position of the qualified area) to indicate the beginning of a field
8 protected and unguarded
9 fill with SPACEs
10 input aligned on the first character position of the qualified area
11 the order of the character positions in the input field is reversed, i.e. the last position in each line becomes the first and vice versa; input begins at the new first position.
This control function operates independently of the setting of the TABULATION STOP MODE (TSM). The character tabulation stop set by parameter value 7 applies to the active line only.
NOTE
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string or an SDS string.
8.3.26 DCH - DELETE CHARACTER
Notation: (Pn) Representation: CSI Pn 05/00
Parameter default value: Pn = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, DCH causes the contents of the active presentation position and, depending on the setting of the CHARACTER EDITING MODE (HEM), the contents of the n-1 preceding or following character positions to be removed from the presentation component, where n equals the value of Pn. The resulting gap is closed by shifting the contents of the adjacent character positions towards the active presentation position. At the other end of the shifted part, n character positions are put into the erased state.
The extent of the shifted part is established by SELECT EDITING EXTENT (SEE).
The effect of DCH on the start or end of a selected area, the start or end of a qualified area, or a tabulation stop in the shifted part is not defined by this Standard.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, DCH causes the contents of the active data position and, depending on the setting of the CHARACTER EDITING MODE (HEM), the contents of the n-1 preceding or following character positions to be removed from the data component, where n equals the value of Pn. The resulting gap is closed by shifting the contents of the adjacent character positions towards the active data position. At the other end of the shifted part, n character positions are put into the erased state.
8.3.27 DCS - DEVICE CONTROL STRING
Notation: (C1)
Representation: 09/00 or ESC 05/00
DCS is used as the opening delimiter of a control string for device control use. The command string following may consist of bit combinations in the range 00/08 to 00/13 and 02/00 to 07/14. The control string is closed by the terminating delimiter STRING TERMINATOR (ST).
The command string represents either one or more commands for the receiving device, or one or more status reports from the sending device. The purpose and the format of the command string are specified by the most recent occurrence of IDENTIFY DEVICE CONTROL STRING (IDCS), if any, or depend on the sending and/or the receiving device.
8.3.28 DC1 - DEVICE CONTROL ONE
Notation: (C0) Representation: 01/01
DC1 is primarily intended for turning on or starting an ancillary device. If it is not required for this purpose, it may be used to restore a device to the basic mode of operation (see also DC2 and DC3), or any other device control function not provided by other DCs.
NOTE
When used for data flow control, DC1 is sometimes called "X-ON".
8.3.29 DC2 - DEVICE CONTROL TWO
Notation: (C0) Representation: 01/02
DC2 is primarily intended for turning on or starting an ancillary device. If it is not required for this purpose, it may be used to set a device to a special mode of operation (in which case DC1 is used to restore the device to the basic mode), or for any other device control function not provided by other DCs.
8.3.30 DC3 - DEVICE CONTROL THREE
Notation: (C0) Representation: 01/03
DC3 is primarily intended for turning off or stopping an ancillary device. This function may be a secondary level stop, for example wait, pause, stand-by or halt (in which case DC1 is used to restore normal operation). If it is not required for this purpose, it may be used for any other device control function not provided by other DCs.
NOTE
When used for data flow control, DC3 is sometimes called "X-OFF".
8.3.31 DC4 - DEVICE CONTROL FOUR
Notation: (C0) Representation: 01/04
DC4 is primarily intended for turning off, stopping or interrupting an ancillary device. If it is not required for this purpose, it may be used for any other device control function not provided by other DCs.
8.3.32 DL - DELETE LINE
Notation: (Pn) Representation: CSI Pn 04/13
Parameter default value: Pn = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, DL causes the contents of the active line (the line that contains the active presentation position) and, depending on the setting of the LINE EDITING MODE (VEM), the contents of the n-1 preceding or following lines to be removed from the presentation component, where n equals the value of Pn. The resulting gap is closed by shifting the contents of a number of adjacent lines towards the active line. At the other end of the shifted part, n lines are put into the erased state.
The active presentation position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH). If the TABULATION STOP MODE (TSM) is set to SINGLE, character tabulation stops are cleared in the lines that are put into the erased state.
The extent of the shifted part is established by SELECT EDITING EXTENT (SEE).
Any occurrences of the start or end of a selected area, the start or end of a qualified area, or a tabulation stop in the shifted part, are also shifted.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, DL causes the contents of the active line (the line that contains the active data position) and, depending on the setting of the LINE EDITING MODE (VEM), the contents of the n-1 preceding or following lines to be removed from the data component, where n equals the value of Pn. The resulting gap is closed by shifting the contents of a number of adjacent lines towards the active line. At the other end of the shifted part, n lines are put into the erased state. The active data position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH).
8.3.33 DLE - DATA LINK ESCAPE
Notation: (C0) Representation: 01/00
DLE is used exclusively to provide supplementary transmission control functions.
The use of DLE is defined in ISO 1745.
8.3.34 DMI - DISABLE MANUAL INPUT
Notation: (Fs) Representation: ESC 06/00
DMI causes the manual input facilities of a device to be disabled.
8.3.35 DSR - DEVICE STATUS REPORT
Notation: (Ps) Representation: CSI Ps 06/14
Parameter default value: Ps = 0
DSR is used either to report the status of the sending device or to request a status report from the receiving device, depending on the parameter values:
0 ready, no malfunction detected
1 busy, another DSR must be requested later
2 busy, another DSR will be sent later
3 some malfunction detected, another DSR must be requested later
4 some malfunction detected, another DSR will be sent later
5 a DSR is requested
6 a report of the active presentation position or of the active data position in the form of ACTIVE POSITION REPORT (CPR) is requested
DSR with parameter value 0, 1, 2, 3 or 4 may be sent either unsolicited or as a response to a request such as a DSR with a parameter value 5 or MESSAGE WAITING (MW).
8.3.36 DTA - DIMENSION TEXT AREA
Notation: (Pn1;Pn2)
Representation: CSI Pn1;Pn2 02/00 05/04
No parameter default value.
DTA is used to establish the dimensions of the text area for subsequent pages.
The established dimensions remain in effect until the next occurrence of DTA in the data stream.
Pn1 specifies the dimension in the direction perpendicular to the line orientation
Pn2 specifies the dimension in the direction parallel to the line orientation
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.37 EA - ERASE IN AREA
Notation: (Ps) Representation: CSI Ps 04/15
Parameter default value: Ps = 0
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, EA causes some or all character positions in the active qualified area (the qualified area in the presentation component which contains the active presentation position) to be put into the erased state, depending on the parameter values:
0 the active presentation position and the character positions up to the end of the qualified area are put into the erased state
1 the character positions from the beginning of the qualified area up to and including the active presentation position are put into the erased state
2 all character positions of the qualified area are put into the erased state
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, EA causes some or all character positions in the active qualified area (the qualified area in the data component which contains the active data position) to be put into the erased state, depending on the parameter values:
0 the active data position and the character positions up to the end of the qualified area are put into the erased state
1 the character positions from the beginning of the qualified area up to and including the active data position are put into the erased state
2 all character positions of the qualified area are put into the erased state
Whether the character positions of protected areas are put into the erased state, or the character positions of unprotected areas only, depends on the setting of the ERASURE MODE (ERM).
8.3.38 ECH - ERASE CHARACTER
Notation: (Pn) Representation: CSI Pn 05/08
Parameter default value: Pn = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, ECH causes the active presentation position and the n-1 following character positions in the presentation component to be put into the erased state, where n equals the value of Pn.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, ECH causes the active data position and the n-1 following character positions in the data component to be put into the erased state, where n equals the value of Pn.
Whether the character positions of protected areas are put into the erased state, or the character positions of unprotected areas only, depends on the setting of the ERASURE MODE (ERM).
8.3.39 ED - ERASE IN PAGE
Notation: (Ps) Representation: CSI Ps 04/10
Parameter default value: Ps = 0
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, ED causes some or all character positions of the active page (the page which contains the active presentation position in the presentation component) to be put into the erased state, depending on the parameter values:
0 the active presentation position and the character positions up to the end of the page are put into the erased state
1 the character positions from the beginning of the page up to and including the active presentation position are put into the erased state
2 all character positions of the page are put into the erased state
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, ED causes some or all character positions of the active page (the page which contains the active data position in the data component) to be put into the erased state, depending on the parameter values:
0 the active data position and the character positions up to the end of the page are put into the erased state
1 the character positions from the beginning of the page up to and including the active data position are put into the erased state
2 all character positions of the page are put into the erased state
Whether the character positions of protected areas are put into the erased state, or the character positions of unprotected areas only, depends on the setting of the ERASURE MODE (ERM).
8.3.40 EF - ERASE IN FIELD
Notation: (Ps) Representation: CSI Ps 04/14
Parameter default value: Ps = 0
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, EF causes some or all character positions of the active field (the field which contains the active presentation position in the presentation component) to be put into the erased state, depending on the parameter values:
0 the active presentation position and the character positions up to the end of the field are put into the erased state
1 the character positions from the beginning of the field up to and including the active presentation position are put into the erased state
2 all character positions of the field are put into the erased state
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, EF causes some or all character positions of the active field (the field which contains the active data position in the data component) to be put into the erased state, depending on the parameter values:
0 the active data position and the character positions up to the end of the field are put into the erased state
1 the character positions from the beginning of the field up to and including the active data position are put into the erased state
2 all character positions of the field are put into the erased state
Whether the character positions of protected areas are put into the erased state, or the character positions of unprotected areas only, depends on the setting of the ERASURE MODE (ERM).
8.3.41 EL - ERASE IN LINE
Notation: (Ps) Representation: CSI Ps 04/11
Parameter default value: Ps = 0
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, EL causes some or all character positions of the active line (the line which contains the active presentation position in the presentation component) to be put into the erased state, depending on the parameter values:
0 the active presentation position and the character positions up to the end of the line are put into the erased state
1 the character positions from the beginning of the line up to and including the active presentation position are put into the erased state
2 all character positions of the line are put into the erased state
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, EL causes some or all character positions of the active line (the line which contains the active data position in the data component) to be put into the erased state, depending on the parameter values:
0 the active data position and the character positions up to the end of the line are put into the erased state
1 the character positions from the beginning of the line up to and including the active data position are put into the erased state
2 all character positions of the line are put into the erased state
Whether the character positions of protected areas are put into the erased state, or the character positions of unprotected areas only, depends on the setting of the ERASURE MODE (ERM).
8.3.42 EM - END OF MEDIUM
Notation: (C0) Representation: 01/09
EM is used to identify the physical end of a medium, or the end of the used portion of a medium, or the end of the wanted portion of data recorded on a medium.
8.3.43 EMI - ENABLE MANUAL INPUT
Notation: (Fs) Representation: ESC 06/02
EMI is used to enable the manual input facilities of a device.
8.3.44 ENQ - ENQUIRY
Notation: (C0) Representation: 00/05
ENQ is transmitted by a sender as a request for a response from a receiver.
The use of ENQ is defined in ISO 1745.
8.3.45 EOT - END OF TRANSMISSION
Notation: (C0) Representation: 00/04
EOT is used to indicate the conclusion of the transmission of one or more texts.
The use of EOT is defined in ISO 1745.
8.3.46 EPA - END OF GUARDED AREA
Notation: (C1)
Representation: 09/07 or ESC 05/07
EPA is used to indicate that the active presentation position is the last of a string of character positions in the presentation component, the contents of which are protected against manual alteration, are guarded against transmission or transfer, depending on the setting of the GUARDED AREA TRANSFER MODE (GATM), and may be protected against erasure, depending on the setting of the ERASURE MODE (ERM). The beginning of this string is indicated by START OF GUARDED AREA (SPA).
NOTE
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string or an SDS string.
8.3.47 ESA - END OF SELECTED AREA
Notation: (C1)
Representation: 08/07 or ESC 04/07
ESA is used to indicate that the active presentation position is the last of a string of character positions in the presentation component, the contents of which are eligible to be transmitted in the form of a data stream or transferred to an auxiliary input/output device. The beginning of this string is indicated by START OF SELECTED AREA (SSA).
NOTE
The control function for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string or an SDS string.
8.3.48 ESC - ESCAPE
Notation: (C0) Representation: 01/11
ESC is used for code extension purposes. It causes the meanings of a limited number of bit combinations following it in the data stream to be changed.
The use of ESC is defined in Standard ECMA-35.
8.3.49 ETB - END OF TRANSMISSION BLOCK
Notation: (C0) Representation: 01/07
ETB is used to indicate the end of a block of data where the data are divided into such blocks for transmission purposes.
The use of ETB is defined in ISO 1745.
8.3.50 ETX - END OF TEXT
Notation: (C0) Representation: 00/03
ETX is used to indicate the end of a text.
The use of ETX is defined in ISO 1745.
8.3.51 FF - FORM FEED
Notation: (C0) Representation: 00/12
FF causes the active presentation position to be moved to the corresponding character position of the line at the page home position of the next form or page in the presentation component. The page home position is established by the parameter value of SET PAGE HOME (SPH).
8.3.52 FNK - FUNCTION KEY
Notation: (Pn)
Representation: CSI Pn 02/00 05/07
No parameter default value.
FNK is a control function in which the parameter value identifies the function key which has been operated.
8.3.53 FNT - FONT SELECTION
Notation: (Ps1;Ps2)
Representation: CSI Ps1;Ps2 02/00 04/04
Parameter default values: Ps1 = 0; Ps2 =0
FNT is used to identify the character font to be selected as primary or alternative font by subsequent occurrences of SELECT GRAPHIC RENDITION (SGR) in the data stream. Ps1 specifies the primary or alternative font concerned:
0 primary font
1 first alternative font
2 second alternative font
3 third alternative font
4 fourth alternative font
5 fifth alternative font
6 sixth alternative font
7 seventh alternative font
8 eighth alternative font
9 ninth alternative font
Ps2 identifies the character font according to a register which is to be established.
8.3.54 GCC - GRAPHIC CHARACTER COMBINATION
Notation: (Ps)
Representation: CSI Ps 02/00 05/15
Parameter default value: Ps = 0
GCC is used to indicate that two or more graphic characters are to be imaged as one single graphic symbol. GCC with a parameter value of 0 indicates that the following two graphic characters are to be imaged as one single graphic symbol; GCC with a parameter value of 1 and GCC with a parameter value of 2 indicate respectively the beginning and the end of a string of graphic characters which are to be imaged as one single graphic symbol.
NOTE
GCC does not explicitly specify the relative sizes or placements of the component parts of a composite graphic symbol. In the simplest case, two components may be "half-width" and side-by-side. For
example, in Japanese text a pair of characters may be presented side-by-side, and occupy the space of a normal-size Kanji character.
8.3.55 GSM - GRAPHIC SIZE MODIFICATION
Notation: (Pn1;Pn2)
Representation: CSI Pn1;Pn2 02/00 04/02
Parameter default values: Pn1 = 100; Pn2 = 100
GSM is used to modify for subsequent text the height and/or the width of all primary and alternative fonts identified by FONT SELECTION (FNT) and established by GRAPHIC SIZE SELECTION (GSS). The established values remain in effect until the next occurrence of GSM or GSS in the data steam.
Pn1 specifies the height as a percentage of the height established by GSS
Pn2 specifies the width as a percentage of the width established by GSS
8.3.56 GSS - GRAPHIC SIZE SELECTION
Notation: (Pn)
Representation: CSI Pn 02/00 04/03
No parameter default value.
GSS is used to establish for subsequent text the height and the width of all primary and alternative fonts identified by FONT SELECTION (FNT). The established values remain in effect until the next occurrence of GSS in the data stream.
Pn specifies the height, the width is implicitly defined by the height.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.57 HPA - CHARACTER POSITION ABSOLUTE
Notation: (Pn) Representation: CSI Pn 06/00
Parameter default value: Pn = 1
HPA causes the active data position to be moved to character position n in the active line (the line in the data component that contains the active data position), where n equals the value of Pn.
8.3.58 HPB - CHARACTER POSITION BACKWARD
Notation: (Pn) Representation: CSI Pn 06/10
Parameter default value: Pn = 1
HPB causes the active data position to be moved by n character positions in the data component in the direction opposite to that of the character progression, where n equals the value of Pn.
8.3.59 HPR - CHARACTER POSITION FORWARD
Notation: (Pn) Representation: CSI Pn 06/01
Parameter default value: Pn = 1
HPR causes the active data position to be moved by n character positions in the data component in the direction of the character progression, where n equals the value of Pn.
8.3.60 HT - CHARACTER TABULATION
Notation: (C0) Representation: 00/09
HT causes the active presentation position to be moved to the following character tabulation stop in the presentation component.
In addition, if that following character tabulation stop has been set by TABULATION ALIGN CENTRE (TAC), TABULATION ALIGN LEADING EDGE (TALE), TABULATION ALIGN TRAILING EDGE (TATE) or TABULATION CENTRED ON CHARACTER (TCC), HT indicates the beginning of a string of text which is to be positioned within a line according to the properties of that tabulation stop. The end of the string is indicated by the next occurrence of HT or CARRIAGE RETURN (CR) or NEXT LINE (NEL) in the data stream.
8.3.61 HTJ - CHARACTER TABULATION WITH JUSTIFICATION
Notation: (C1)
Representation: 08/09 or ESC 04/09
HTJ causes the contents of the active field (the field in the presentation component that contains the active presentation position) to be shifted forward so that it ends at the character position preceding the following character tabulation stop. The active presentation position is moved to that following character tabulation stop. The character positions which precede the beginning of the shifted string are put into the erased state.
8.3.62 HTS - CHARACTER TABULATION SET
Notation: (C1)
Representation: 08/08 or ESC 04/08
HTS causes a character tabulation stop to be set at the active presentation position in the presentation component.
The number of lines affected depends on the setting of the TABULATION STOP MODE (TSM).
8.3.63 HVP - CHARACTER AND LINE POSITION
Notation: (Pn1;Pn2) Representation: CSI Pn1;Pn2 06/06
Parameter default values: Pn1 = 1; Pn2 = 1
HVP causes the active data position to be moved in the data component to the n-th line position according to the line progression and to the m-th character position according to the character progression, where n equals the value of Pn1 and m equals the value of Pn2.
8.3.64 ICH - INSERT CHARACTER
Notation: (Pn) Representation: CSI Pn 04/00
Parameter default value: Pn = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, ICH is used to prepare the insertion of n characters, by putting into the erased state the active presentation position and, depending on the setting of the CHARACTER EDITING MODE (HEM), the n-1 preceding or following character positions in the presentation component, where n equals the value of Pn. The previous contents of the active presentation position and an adjacent string of character positions are shifted away from the active presentation position. The contents of n character positions at the other end of the shifted part are removed. The active presentation position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH).
The extent of the shifted part is established by SELECT EDITING EXTENT (SEE).
The effect of ICH on the start or end of a selected area, the start or end of a qualified area, or a tabulation stop in the shifted part, is not defined by this Standard.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, ICH is used to prepare the insertion of n characters, by putting into the erased state the active data position and, depending on the setting of the CHARACTER EDITING MODE (HEM), the n-1 preceding or following character positions in the data component, where n equals the value of Pn. The previous contents of the active data position and an adjacent string of character positions are shifted away from the active data position. The contents of n character positions at the other end of the shifted part are removed. The active data
position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH).
8.3.65 IDCS - IDENTIFY DEVICE CONTROL STRING
Notation: (Ps)
Representation: CSI Ps 02/00 04/15
No parameter default value.
IDCS is used to specify the purpose and format of the command string of subsequent DEVICE CONTROL STRINGs (DCS). The specified purpose and format remain in effect until the next occurrence of IDCS in the data stream.
The parameter values are
1 reserved for use with the DIAGNOSTIC state of the STATUS REPORT TRANSFER MODE (SRTM)
2 reserved for Dynamically Redefinable Character Sets (DRCS) according to Standard ECMA-35.
The format and interpretation of the command string corresponding to these parameter values are to be defined in appropriate standards. If this control function is used to identify a private command string, a private parameter value shall be used.
8.3.66 IGS - IDENTIFY GRAPHIC SUBREPERTOIRE
Notation: (Ps)
Representation: CSI Ps 02/00 04/13
No parameter default value.
IGS is used to indicate that a repertoire of the graphic characters of ISO/IEC 10367 is used in the subsequent text.
The parameter value of IGS identifies a graphic character repertoire registered in accordance with ISO/IEC 7350.
8.3.67 IL - INSERT LINE
Notation: (Pn) Representation: CSI Pn 04/12
Parameter default value: Pn = 1
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, IL is used to prepare the insertion of n lines, by putting into the erased state in the presentation component the active line (the line that contains the active presentation position) and, depending on the setting of the LINE EDITING MODE (VEM), the n-1 preceding or following lines, where n equals the value of Pn. The previous contents of the active line and of adjacent lines are shifted away from the active line. The contents of n lines at the other end of the shifted part are removed. The active presentation position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH).
The extent of the shifted part is established by SELECT EDITING EXTENT (SEE).
Any occurrences of the start or end of a selected area, the start or end of a qualified area, or a tabulation stop in the shifted part, are also shifted.
If the TABULATION STOP MODE (TSM) is set to SINGLE, character tabulation stops are cleared in the lines that are put into the erased state.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, IL is used to prepare the insertion of n lines, by putting into the erased state in the data component the active line (the line that contains the active data position) and, depending on the setting of the LINE EDITING MODE (VEM), the n-1 preceding or following lines, where n equals the value of Pn. The previous contents of the active line and of adjacent lines are shifted away from the active line. The contents of n lines at the other end of the shifted part are removed. The active data position is moved to the line home position in the active line. The line home position is established by the parameter value of SET LINE HOME (SLH).
8.3.68 INT - INTERRUPT
Notation: (Fs) Representation: ESC 06/01
INT is used to indicate to the receiving device that the current process is to be interrupted and an agreed procedure is to be initiated. This control function is applicable to either direction of transmission.
8.3.69 IS1 - INFORMATION SEPARATOR ONE (US - UNIT SEPARATOR)
Notation: (C0) Representation: 01/15
IS1 is used to separate and qualify data logically; its specific meaning has to be defined for each application. If this control function is used in hierarchical order, it may delimit a data item called a unit, see 8.2.10.
8.3.70 IS2 - INFORMATION SEPARATOR TWO (RS - RECORD SEPARATOR)
Notation: (C0) Representation: 01/14
IS2 is used to separate and qualify data logically; its specific meaning has to be defined for each application. If this control function is used in hierarchical order, it may delimit a data item called a record, see 8.2.10.
8.3.71 IS3 - INFORMATION SEPARATOR THREE (GS - GROUP SEPARATOR)
Notation: (C0) Representation: 01/13
IS3 is used to separate and qualify data logically; its specific meaning has to be defined for each application. If this control function is used in hierarchical order, it may delimit a data item called a group, see 8.2.10.
8.3.72 IS4 - INFORMATION SEPARATOR FOUR (FS - FILE SEPARATOR)
Notation: (C0) Representation: 01/12
IS4 is used to separate and qualify data logically; its specific meaning has to be defined for each application. If this control function is used in hierarchical order, it may delimit a data item called a file, see 8.2.10.
8.3.73 JFY - JUSTIFY
Notation: (Ps...)
Representation: CSI Ps... 02/00 04/06
Parameter default value: Ps = 0
JFY is used to indicate the beginning of a string of graphic characters in the presentation component that are to be justified according to the layout specified by the parameter values, see annex C:
0 no justification, end of justification of preceding text
1 word fill
2 word space
3 letter space
4 hyphenation
5 flush to line home position margin
6 centre between line home position and line limit position margins
7 flush to line limit position margin
8 Italian hyphenation
The end of the string to be justified is indicated by the next occurrence of JFY in the data stream.
The line home position is established by the parameter value of SET LINE HOME (SLH). The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
8.3.74 LF - LINE FEED
Notation: (C0) Representation: 00/10
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, LF causes the active presentation position to be moved to the corresponding character position of the following line in the presentation component.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, LF causes the active data position to be moved to the corresponding character position of the following line in the data component.
8.3.75 LS0 - LOCKING-SHIFT ZERO
Notation: (C0) Representation: 00/15
LS0 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS0 is defined in Standard ECMA-35.
NOTE
LS0 is used in 8-bit environments only; in 7-bit environments SHIFT-IN (SI) is used instead.
8.3.76 LS1 - LOCKING-SHIFT ONE
Notation: (C0) Representation: 00/14
LS1 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS1 is defined in Standard ECMA-35.
NOTE
LS1 is used in 8-bit environments only; in 7-bit environments SHIFT-OUT (SO) is used instead.
8.3.77 LS1R - LOCKING-SHIFT ONE RIGHT
Notation: (Fs) Representation: ESC 07/14
LS1R is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS1R is defined in Standard ECMA-35.
8.3.78 LS2 - LOCKING-SHIFT TWO
Notation: (Fs) Representation: ESC 06/14
LS2 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS2 is defined in Standard ECMA-35.
8.3.79 LS2R - LOCKING-SHIFT TWO RIGHT
Notation: (Fs) Representation: ESC 07/13
LS2R is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS2R is defined in Standard ECMA-35.
8.3.80 LS3 - LOCKING-SHIFT THREE
Notation: (Fs) Representation: ESC 06/15
LS3 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS3 is defined in Standard ECMA-35.
8.3.81 LS3R - LOCKING-SHIFT THREE RIGHT
Notation: (Fs) Representation: ESC 07/12
LS3R is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of LS3R is defined in Standard ECMA-35.
8.3.82 MC - MEDIA COPY
Notation: (Ps) Representation: CSI Ps 06/09
Parameter default value: Ps = 0
MC is used either to initiate a transfer of data from or to an auxiliary input/output device or to enable or disable the relay of the received data stream to an auxiliary input/output device, depending on the parameter value:
0 initiate transfer to a primary auxiliary device
1 initiate transfer from a primary auxiliary device
2 initiate transfer to a secondary auxiliary device
3 initiate transfer from a secondary auxiliary device
4 stop relay to a primary auxiliary device
5 start relay to a primary auxiliary device
6 stop relay to a secondary auxiliary device
7 start relay to a secondary auxiliary device
This control function may not be used to switch on or off an auxiliary device.
8.3.83 MW - MESSAGE WAITING
Notation: (C1)
Representation: 09/05 or ESC 05/05
MW is used to set a message waiting indicator in the receiving device. An appropriate acknowledgement to the receipt of MW may be given by using DEVICE STATUS REPORT (DSR).
8.3.84 NAK - NEGATIVE ACKNOWLEDGE
Notation: (C0) Representation: 01/05
NAK is transmitted by a receiver as a negative response to the sender.
The use of NAK is defined in ISO 1745.
8.3.85 NBH - NO BREAK HERE
Notation: (C1)
Representation: 08/03 or ESC 04/03
NBH is used to indicate a point where a line break shall not occur when text is formatted. NBH may occur between two graphic characters either or both of which may be SPACE.
8.3.86 NEL - NEXT LINE
Notation: (C1)
Representation: 08/05 or ESC 04/05
The effect of NEL depends on the setting of the DEVICE COMPONENT SELECT MODE (DCSM) and on the parameter value of SELECT IMPLICIT MOVEMENT DIRECTION (SIMD).
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION and with a parameter value of SIMD equal to 0, NEL causes the active presentation position to be moved to the line home position of the following line in the presentation component. The line home position is established by the parameter value of SET LINE HOME (SLH).
With a parameter value of SIMD equal to 1, NEL causes the active presentation position to be moved to the line limit position of the following line in the presentation component. The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA and with a parameter value of SIMD equal to 0, NEL causes the active data position to be moved to the line home position of the following line in the data component. The line home position is established by the parameter value of SET LINE HOME (SLH).
With a parameter value of SIMD equal to 1, NEL causes the active data position to be moved to the line limit position of the following line in the data component. The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
8.3.87 NP - NEXT PAGE
Notation: (Pn) Representation: CSI Pn 05/05
Parameter default value: Pn = 1
NP causes the n-th following page in the presentation component to be displayed, where n equals the value of Pn.
The effect of this control function on the active presentation position is not defined by this Standard.
8.3.88 NUL - NULL
Notation: (C0) Representation: 00/00
NUL is used for media-fill or time-fill. NUL characters may be inserted into, or removed from, a data stream without affecting the information content of that stream, but such action may affect the information layout and/or the control of equipment.
8.3.89 OSC - OPERATING SYSTEM COMMAND
Notation: (C1)
Representation: 09/13 or ESC 05/13
OSC is used as the opening delimiter of a control string for operating system use. The command string following may consist of a sequence of bit combinations in the range 00/08 to 00/13 and 02/00 to 07/14. The control string is closed by the terminating delimiter STRING TERMINATOR (ST). The interpretation of the command string depends on the relevant operating system.
8.3.90 PEC - PRESENTATION EXPAND OR CONTRACT
Notation: (Ps)
Representation: CSI Ps 02/00 05/10
Parameter default value: Ps = 0
PEC is used to establish the spacing and the extent of the graphic characters for subsequent text. The spacing is specified in the line as multiples of the spacing established by the most recent occurrence of SET CHARACTER SPACING (SCS) or of SELECT CHARACTER SPACING (SHS) or of SPACING INCREMENT (SPI) in the data stream. The extent of the characters is implicitly established by these
control functions. The established spacing and the extent remain in effect until the next occurrence of PEC, of SCS, of SHS or of SPI in the data stream. The parameter values are
0 normal (as specified by SCS, SHS or SPI)
1 expanded (multiplied by a factor not greater than 2)
2 condensed (multiplied by a factor not less than 0,5)
8.3.91 PFS - PAGE FORMAT SELECTION
Notation: (Ps)
Representation: CSI Ps 02/00 04/10
Parameter default value: Ps = 0
PFS is used to establish the available area for the imaging of pages of text based on paper size. The pages are introduced by the subsequent occurrence of FORM FEED (FF) in the data stream.
The established image area remains in effect until the next occurrence of PFS in the data stream. The parameter values are (see also annex E):
0 tall basic text communication format
1 wide basic text communication format
2 tall basic A4 format
3 wide basic A4 format
4 tall North American letter format
5 wide North American letter format
6 tall extended A4 format
7 wide extended A4 format
8 tall North American legal format
9 wide North American legal format
10 A4 short lines format
11 A4 long lines format
12 B5 short lines format
13 B5 long lines format
14 B4 short lines format
15 B4 long lines format
The page home position is established by the parameter value of SET PAGE HOME (SPH), the page limit position is established by the parameter value of SET PAGE LIMIT (SPL).
8.3.92 PLD - PARTIAL LINE FORWARD
Notation: (C1)
Representation: 08/11 or ESC 04/11
PLD causes the active presentation position to be moved in the presentation component to the corresponding position of an imaginary line with a partial offset in the direction of the line progression. This offset should be sufficient either to image following characters as subscripts until the first following occurrence of PARTIAL LINE BACKWARD (PLU) in the data stream, or, if preceding characters were imaged as superscripts, to restore imaging of following characters to the active line (the line that contains the active presentation position).
Any interactions between PLD and format effectors other than PLU are not defined by this Standard.
8.3.93 PLU - PARTIAL LINE BACKWARD
Notation: (C1)
Representation: 08/12 or ESC 04/12
PLU causes the active presentation position to be moved in the presentation component to the corresponding position of an imaginary line with a partial offset in the direction opposite to that of the line progression. This offset should be sufficient either to image following characters as superscripts until the first following occurrence of PARTIAL LINE FORWARD (PLD) in the data stream, or, if preceding characters were imaged as subscripts, to restore imaging of following characters to the active line (the line that contains the active presentation position).
Any interactions between PLU and format effectors other than PLD are not defined by this Standard.
8.3.94 PM - PRIVACY MESSAGE
Notation: (C1)
Representation: 09/14 or ESC 05/14
PM is used as the opening delimiter of a control string for privacy message use. The command string following may consist of a sequence of bit combinations in the range 00/08 to 00/13 and 02/00 to 07/14. The control string is closed by the terminating delimiter STRING TERMINATOR (ST). The interpretation of the command string depends on the relevant privacy discipline.
8.3.95 PP - PRECEDING PAGE
Notation: (Pn) Representation: CSI Pn 05/06
Parameter default value: Pn = 1
PP causes the n-th preceding page in the presentation component to be displayed, where n equals the value of Pn. The effect of this control function on the active presentation position is not defined by this Standard.
8.3.96 PPA - PAGE POSITION ABSOLUTE
Notation: (Pn)
Representation: CSI Pn 02/00 05/00
Parameter default value: Pn = 1
PPA causes the active data position to be moved in the data component to the corresponding character position on the n-th page, where n equals the value of Pn.
8.3.97 PPB - PAGE POSITION BACKWARD
Notation: (Pn)
Representation: CSI Pn 02/00 05/02
Parameter default value: Pn = 1
PPB causes the active data position to be moved in the data component to the corresponding character position on the n-th preceding page, where n equals the value of Pn.
8.3.98 PPR - PAGE POSITION FORWARD
Notation: (Pn)
Representation: CSI Pn 02/00 05/01
Parameter default value: Pn = 1
PPR causes the active data position to be moved in the data component to the corresponding character position on the n-th following page, where n equals the value of Pn.
8.3.99 PTX - PARALLEL TEXTS
Notation: (Ps) Representation: CSI Ps 05/12
Parameter default value: Ps = 0
PTX is used to delimit strings of graphic characters that are communicated one after another in the data stream but that are intended to be presented in parallel with one another, usually in adjacent lines.
The parameter values are
0 end of parallel texts
1 beginning of a string of principal parallel text
2 beginning of a string of supplementary parallel text
3 beginning of a string of supplementary Japanese phonetic annotation
4 beginning of a string of supplementary Chinese phonetic annotation
5 end of a string of supplementary phonetic annotations
PTX with a parameter value of 1 indicates the beginning of the string of principal text intended to be presented in parallel with one or more strings of supplementary text.
PTX with a parameter value of 2, 3 or 4 indicates the beginning of a string of supplementary text that is intended to be presented in parallel with either a string of principal text or the immediately preceding string of supplementary text, if any; at the same time it indicates the end of the preceding string of principal text or of the immediately preceding string of supplementary text, if any. The end of a string of supplementary text is indicated by a subsequent occurrence of PTX with a parameter value other than 1.
PTX with a parameter value of 0 indicates the end of the strings of text intended to be presented in parallel with one another.
NOTE
PTX does not explicitly specify the relative placement of the strings of principal and supplementary parallel texts, or the relative sizes of graphic characters in the strings of parallel text. A string of supplementary text is normally presented in a line adjacent to the line containing the string of principal text, or adjacent to the line containing the immediately preceding string of supplementary text, if any. The first graphic character of the string of principal text and the first graphic character of a string of supplementary text are normally presented in the same position of their respective lines. However, a string of supplementary text longer (when presented) than the associated string of principal text may be centred on that string. In the case of long strings of text, such as paragraphs in different languages, the strings may be presented in successive lines in parallel columns, with their beginnings aligned with one another and the shorter of the paragraphs followed by an appropriate amount of "white space".
Japanese phonetic annotation typically consists of a few half-size or smaller Kana characters which indicate the pronunciation or interpretation of one or more Kanji characters and are presented above those Kanji characters if the character path is horizontal, or to the right of them if the character path is vertical.
Chinese phonetic annotation typically consists of a few Pinyin characters which indicate the pronunciation of one or more Hanzi characters and are presented above those Hanzi characters. Alternatively, the Pinyin characters may be presented in the same line as the Hanzi characters and following the respective Hanzi characters. The Pinyin characters will then be presented within enclosing pairs of parentheses.
8.3.100 PU1 - PRIVATE USE ONE
Notation: (C1)
Representation: 09/01 or ESC 05/01
PU1 is reserved for a function without standardized meaning for private use as required, subject to the prior agreement between the sender and the recipient of the data.
8.3.101 PU2 - PRIVATE USE TWO
Notation: (C1)
Representation: 09/02 or ESC 05/02
PU2 is reserved for a function without standardized meaning for private use as required, subject to the prior agreement between the sender and the recipient of the data.
8.3.102 QUAD - QUAD
Notation: (Ps...)
Representation: CSI Ps... 02/00 04/08
Parameter default value: Ps = 0
QUAD is used to indicate the end of a string of graphic characters that are to be positioned on a single line according to the layout specified by the parameter values, see annex C:
0 flush to line home position margin
1 flush to line home position margin and fill with leader
2 centre between line home position and line limit position margins
3 centre between line home position and line limit position margins and fill with leader
4 flush to line limit position margin
5 flush to line limit position margin and fill with leader
6 flush to both margins
The beginning of the string to be positioned is indicated by the preceding occurrence in the data stream of either QUAD or one of the following formator functions: FORM FEED (FF), CHARACTER AND LINE POSITION (HVP), LINE FEED (LF), NEXT LINE (NEL), PAGE POSITION ABSOLUTE (PPA), PAGE POSITION BACKWARD (PPB), PAGE POSITION FORWARD (PPR), REVERSE LINE FEED (RI), LINE POSITION ABSOLUTE (VPA), LINE POSITION BACKWARD (VPB), LINE POSITION FORWARD (VPR), or LINE TABULATION (VT).
The line home position is established by the parameter value of SET LINE HOME (SLH). The line limit position is established by the parameter value of SET LINE LIMIT (SLL).
8.3.103 REP - REPEAT
Notation: (Pn) Representation: CSI Pn 06/02
Parameter default value: Pn = 1
REP is used to indicate that the preceding character in the data stream, if it is a graphic character (represented by one or more bit combinations) including SPACE, is to be repeated n times, where n equals the value of Pn. If the character preceding REP is a control function or part of a control function, the effect of REP is not defined by this Standard.
8.3.104 RI - REVERSE LINE FEED
Notation: (C1)
Representation: 08/13 or ESC 04/13
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to PRESENTATION, RI causes the active presentation position to be moved in the presentation component to the corresponding character position of the preceding line.
If the DEVICE COMPONENT SELECT MODE (DCSM) is set to DATA, RI causes the active data position to be moved in the data component to the corresponding character position of the preceding line.
8.3.105 RIS - RESET TO INITIAL STATE
Notation: (Fs) Representation: ESC 06/03
RIS causes a device to be reset to its initial state, i.e. the state it has after it is made operational. This may imply, if applicable: clear tabulation stops, remove qualified areas, reset graphic rendition, put all character positions into the erased state, move the active presentation position to the first position of the first line in the presentation component, move the active data position to the first character position of the first line in the data component, set the modes into the reset state, etc.
8.3.106 RM - RESET MODE
Notation: (Ps...) Representation: CSI Ps... 06/12
No parameter default value.
RM causes the modes of the receiving device to be reset as specified by the parameter values:
1 GUARDED AREA TRANSFER MODE (GATM)
2 KEYBOARD ACTION MODE (KAM)
3 CONTROL REPRESENTATION MODE (CRM)
4 INSERTION REPLACEMENT MODE (IRM)
5 STATUS REPORT TRANSFER MODE (SRTM)
6 ERASURE MODE (ERM)
7 LINE EDITING MODE (VEM)
8 BI-DIRECTIONAL SUPPORT MODE (BDSM)
9 DEVICE COMPONENT SELECT MODE (DCSM)
10 CHARACTER EDITING MODE (HEM)
11 POSITIONING UNIT MODE (PUM) (see F.4.1 in annex F)
12 SEND/RECEIVE MODE (SRM)
13 FORMAT EFFECTOR ACTION MODE (FEAM)
14 FORMAT EFFECTOR TRANSFER MODE (FETM)
15 MULTIPLE AREA TRANSFER MODE (MATM)
16 TRANSFER TERMINATION MODE (TTM)
17 SELECTED AREA TRANSFER MODE (SATM)
18 TABULATION STOP MODE (TSM)
19 (Shall not be used; see F.5.1 in annex F)
20 (Shall not be used; see F.5.2 in annex F)
21 GRAPHIC RENDITION COMBINATION MODE (GRCM)
22 ZERO DEFAULT MODE (ZDM) (see F.4.2 in annex F)
NOTE
Private modes may be implemented using private parameters, see 5.4.1 and 7.4.
8.3.107 SACS - SET ADDITIONAL CHARACTER SEPARATION
Notation: (Pn)
Representation: CSI Pn 02/00 05/12
Parameter default value: Pn = 0
SACS is used to establish extra inter-character escapement for subsequent text. The established extra escapement remains in effect until the next occurrence of SACS or of SET REDUCED CHARACTER SEPARATION (SRCS) in the data stream or until it is reset to the default value by a subsequent occurrence of CARRIAGE RETURN/LINE FEED (CR LF) or of NEXT LINE (NEL) in the data stream, see annex C.
Pn specifies the number of units by which the inter-character escapement is enlarged.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.108 SAPV - SELECT ALTERNATIVE PRESENTATION VARIANTS
Notation: (Ps...)
Representation: CSI Ps... 02/00 05/13
Parameter default value: Ps = 0
SAPV is used to specify one or more variants for the presentation of subsequent text. The parameter values are
0 default presentation (implementation-defined); cancels the effect of any preceding occurrence of SAPV in the data stream
1 the decimal digits are presented by means of the graphic symbols used in the Latin script
2 the decimal digits are presented by means of the graphic symbols used in the Arabic script, i.e. the Hindi symbols
3 when the direction of the character path is right-to-left, each of the graphic characters in the graphic character set(s) in use which is one of a left/right-handed pair (parentheses, square brackets, curly brackets, greater-than/less-than signs, etc.) is presented as "mirrored", i.e. as the other member of the pair. For example, the coded graphic character given the name LEFT PARENTHESIS is presented as RIGHT PARENTHESIS, and vice versa
4 when the direction of the character path is right-to-left, all graphic characters which represent operators and delimiters in mathematical formulae and which are not symmetrical about a vertical axis are presented as mirrored about that vertical axis
5 the following graphic character is presented in its isolated form
6 the following graphic character is presented in its initial form
7 the following graphic character is presented in its medial form
8 the following graphic character is presented in its final form
9 where the bit combination 02/14 is intended to represent a decimal mark in a decimal number it shall be presented by means of the graphic symbol FULL STOP
10 where the bit combination 02/14 is intended to represent a decimal mark in a decimal number it shall be presented by means of the graphic symbol COMMA
11 vowels are presented above or below the preceding character
12 vowels are presented after the preceding character
13 contextual shape determination of Arabic scripts, including the LAM-ALEPH ligature but excluding all other Arabic ligatures
14 contextual shape determination of Arabic scripts, excluding all Arabic ligatures
15 cancels the effect of parameter values 3 and 4
16 vowels are not presented
17 when the string direction is right-to-left, the italicized characters are slanted to the left; when the string direction is left-to-right, the italicized characters are slanted to the right
18 contextual shape determination of Arabic scripts is not used, the graphic characters - including the digits - are presented in the form they are stored (Pass-through)
19 contextual shape determination of Arabic scripts is not used, the graphic characters- excluding the digits - are presented in the form they are stored (Pass-through)
20 the graphic symbols used to present the decimal digits are device dependent
21 establishes the effect of parameter values 5, 6, 7, and 8 for the following graphic characters until cancelled
22 cancels the effect of parameter value 21, i.e. re-establishes the effect of parameter values 5, 6, 7, and 8 for the next single graphic character only.
8.3.109 SCI - SINGLE CHARACTER INTRODUCER
Notation: (C1)
Representation: 09/10 or ESC 05/10
SCI and the bit combination following it are used to represent a control function or a graphic character. The bit combination following SCI must be from 00/08 to 00/13 or 02/00 to 07/14. The use of SCI is reserved for future standardization.
8.3.110 SCO - SELECT CHARACTER ORIENTATION
Notation: (Ps)
Representation: CSI Ps 02/00 06/05
Parameter default value: Ps = 0
SCO is used to establish the amount of rotation of the graphic characters following in the data stream. The established value remains in effect until the next occurrence of SCO in the data stream.
The parameter values are 0 0°
1 45°
2 90°
3 135° 4 180° 5 225° 6 270° 7 315°
is positive, i.e. counter-clockwise and applies to the normal presentation of the graphic
Rotation
characters along the character path. The centre of rotation of the affected graphic characters is not defined by this Standard.
8.3.111 SCP - SELECT CHARACTER PATH
Notation: (Ps1;Ps2)
Representation: CSI Ps1;Ps2 02/00 06/11
No parameter default values.
SCP is used to select the character path, relative to the line orientation, for the active line (the line that contains the active presentation position) and subsequent lines in the presentation component. It is also used to update the content of the active line in the presentation component and the content of the active line (the line that contains the active data position) in the data component. This takes effect immediately.
Ps1 specifies the character path:
1 left-to-right (in the case of horizontal line orientation), or top-to-bottom (in the case of vertical line orientation)
2 right-to-left (in the case of horizontal line orientation), or bottom-to-top (in the case of vertical line orientation)
Ps2 specifies the effect on the content of the presentation component and the content of the data component:
0 undefined (implementation-dependent)
NOTE
This may also permit the effect to take place after the next occurrence of CR, NEL or any control function which initiates an absolute movement of the active presentation position or the active data position.
1 the content of the active line in the presentation component (the line that contains the active presentation position) is updated to correspond to the content of the active line in the data component (the line that contains the active data position) according to the newly established character path characteristics in the presentation component; the active data position is moved to the first character position in the active line in the data component, the active presentation position in the presentation component is updated accordingly
2 the content of the active line in the data component (the line that contains the active data position) is updated to correspond to the content of the active line in the presentation component (the line that contains the active presentation position) according to the newly established character path characteristics of the presentation component; the active presentation position is moved to the first character position in the active line in the presentation component, the active data position in the data component is updated accordingly.
8.3.112 SCS - SET CHARACTER SPACING
Notation: (Pn)
Representation: CSI Pn 02/00 06/07
No parameter default value.
SCS is used to establish the character spacing for subsequent text. The established spacing remains in effect until the next occurrence of SCS, or of SELECT CHARACTER SPACING (SHS) or of SPACING INCREMENT (SPI) in the data stream, see annex C.
Pn specifies the character spacing.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.113 SD - SCROLL DOWN
Notation: (Pn) Representation: CSI Pn 05/04
Parameter default value: Pn = 1
SD causes the data in the presentation component to be moved by n line positions if the line orientation is horizontal, or by n character positions if the line orientation is vertical, such that the data appear to move down; where n equals the value of Pn.
The active presentation position is not affected by this control function.
8.3.114 SDS - START DIRECTED STRING
Notation: (Ps) Representation: CSI Ps 05/13
Parameter default value: Ps = 0
SDS is used to establish in the data component the beginning and the end of a string of characters as well as the direction of the string. This direction may be different from that currently established. The indicated string follows the preceding text. The established character progression is not affected.
The beginning of a directed string is indicated by SDS with a parameter value not equal to 0. A directed string may contain one or more nested strings. These nested strings may be directed strings the beginnings of which are indicated by SDS with a parameter value not equal to 0, or reversed strings the beginnings of which are indicated by START REVERSED STRING (SRS) with a parameter value of 1. Every beginning of such a string invokes the next deeper level of nesting.
This Standard does not define the location of the active data position within any such nested string.
The end of a directed string is indicated by SDS with a parameter value of 0. Every end of such a string re-establishes the next higher level of nesting (the one in effect prior to the string just ended). The direction is re-established to that in effect prior to the string just ended. The active data position is moved to the character position following the characters of the string just ended.
The parameter values are:
0 end of a directed string; re-establish the previous direction
1 start of a directed string; establish the direction left-to-right
2 start of a directed string; establish the direction right-to-left
NOTE 1
The effect of receiving a CVT, HT, SCP, SPD or VT control function within an SDS string is not defined by this Standard.
NOTE 2
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SDS string.
8.3.115 SEE - SELECT EDITING EXTENT
Notation: (Ps) Representation: CSI Ps 05/01
Parameter default value: Ps = 0
SEE is used to establish the editing extent for subsequent character or line insertion or deletion. The established extent remains in effect until the next occurrence of SEE in the data stream. The editing extent depends on the parameter value:
0 the shifted part is limited to the active page in the presentation component
1 the shifted part is limited to the active line in the presentation component
2 the shifted part is limited to the active field in the presentation component
3 the shifted part is limited to the active qualified area
4 the shifted part consists of the relevant part of the entire presentation component.
8.3.116 SEF - SHEET EJECT AND FEED
Notation: (Ps1;Ps2)
Representation: CSI Ps1;Ps2 02/00 05/09
Parameter default values: Ps1 = 0; Ps2 = 0
SEF causes a sheet of paper to be ejected from a printing device into a specified output stacker and another sheet to be loaded into the printing device from a specified paper bin.
Parameter values of Ps1 are:
0 eject sheet, no new sheet loaded
1 eject sheet and load another from bin 1
2 eject sheet and load another from bin 2
. . .
n eject sheet and load another from bin n Parameter values of Ps2 are:
0 eject sheet, no stacker specified
1 eject sheet into stacker 1
2 eject sheet into stacker 2
. . .
n eject sheet into stacker n
8.3.117 SGR - SELECT GRAPHIC RENDITION
Notation: (Ps...) Representation: CSI Ps... 06/13
Parameter default value: Ps = 0
SGR is used to establish one or more graphic rendition aspects for subsequent text. The established aspects remain in effect until the next occurrence of SGR in the data stream, depending on the setting of the GRAPHIC RENDITION COMBINATION MODE (GRCM). Each graphic rendition aspect is specified by a parameter value:
0 default rendition (implementation-defined), cancels the effect of any preceding occurrence of SGR in the data stream regardless of the setting of the GRAPHIC RENDITION COMBINATION MODE (GRCM)
1 bold or increased intensity
2 faint, decreased intensity or second colour
3 italicized
4 singly underlined
5 slowly blinking (less then 150 per minute)
6 rapidly blinking (150 per minute or more)
7 negative image
8 concealed characters
9 crossed-out (characters still legible but marked as to be deleted)
10 primary (default) font
11 first alternative font
12 second alternative font
13 third alternative font
14 fourth alternative font
15 fifth alternative font
16 sixth alternative font
17 seventh alternative font
18 eighth alternative font
19 ninth alternative font
20 Fraktur (Gothic)
21 doubly underlined
22 normal colour or normal intensity (neither bold nor faint)
23 not italicized, not fraktur
24 not underlined (neither singly nor doubly)
25 steady (not blinking)
26 (reserved for proportional spacing as specified in CCITT Recommendation T.61)
27 positive image
28 revealed characters
29 not crossed out 30 black display 31 red display
32 green display 33 yellow display 34 blue display
35 magenta display
36 cyan display
37 white display
38 (reserved for future standardization; intended for setting character foreground colour as specified in ISO 8613-6 [CCITT Recommendation T.416])
39 default display colour (implementation-defined) 40 black background
41 red background
42 green background
43 yellow background 44 blue background
45 magenta background 46 cyan background
47 white background
48 (reserved for future standardization; intended for setting character background colour as specified in ISO 8613-6 [CCITT Recommendation T.416])
49 default background colour (implementation-defined)
50 (reserved for cancelling the effect of the rendering aspect established by parameter value 26) 51 framed
52 encircled
53 overlined
54 not framed, not encircled
55 not overlined
56 (reserved for future standardization)
57 (reserved for future standardization)
58 (reserved for future standardization)
59 (reserved for future standardization)
60 ideogram underline or right side line
61 ideogram double underline or double line on the right side
62 ideogram overline or left side line
63 ideogram double overline or double line on the left side
64 ideogram stress marking
65 cancels the effect of the rendition aspects established by parameter values 60 to 64
0 1 2 3 4 5 6
10 characters per 25,4 mm 12 characters per 25,4 mm 15 characters per 25,4 mm
6 characters per 25,4 mm 3 characters per 25,4 mm 9 characters per 50,8 mm 4 characters per 25,4 mm
NOTE
The usable combinations of parameter values are determined by the implementation.
8.3.118 SHS - SELECT CHARACTER SPACING
Notation: (Ps)
Representation: CSI Ps 02/00 04/11
Parameter default value: Ps = 0
SHS is used to establish the character spacing for subsequent text. The established spacing remains in effect until the next occurrence of SHS or of SET CHARACTER SPACING (SCS) or of SPACING INCREMENT (SPI) in the data stream. The parameter values are
8.3.119 SI - SHIFT-IN
Notation: (C0) Representation: 00/15
SI is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of SI is defined in Standard ECMA-35.
NOTE
SI is used in 7-bit environments only; in 8-bit environments LOCKING-SHIFT ZERO (LS0) is used instead.
8.3.120 SIMD - SELECT IMPLICIT MOVEMENT DIRECTION
Notation: (Ps) Representation: CSI Ps 05/14
Parameter default value: Ps = 0
SIMD is used to select the direction of implicit movement of the data position relative to the character progression. The direction selected remains in effect until the next occurrence of SIMD.
The parameter values are:
0 the direction of implicit movement is the same as that of the character progression
1 the direction of implicit movement is opposite to that of the character progression.
8.3.121 SL - SCROLL LEFT
Notation: (Pn)
Representation: CSI Pn 02/00 04/00
Parameter default value: Pn = 1
SL causes the data in the presentation component to be moved by n character positions if the line orientation is horizontal, or by n line positions if the line orientation is vertical, such that the data appear to move to the left; where n equals the value of Pn.
The active presentation position is not affected by this control function.
8.3.122 SLH - SET LINE HOME
Notation: (Pn)
Representation: CSI Pn 02/00 05/05
No parameter default value.
If the DEVICE COMPONENT SELECT MODE is set to PRESENTATION, SLH is used to establish at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component the position to which the active presentation position will be moved by subsequent occurrences of CARRIAGE RETURN (CR), DELETE LINE (DL), INSERT LINE (IL) or NEXT LINE (NEL) in the data stream; where n equals the value of Pn. In the case of a device without data component, it is also the position ahead of which no implicit movement of the active presentation position shall occur.
If the DEVICE COMPONENT SELECT MODE is set to DATA, SLH is used to establish at character position n in the active line (the line that contains the active data position) and lines of subsequent text in the data component the position to which the active data position will be moved by subsequent occurrences of CARRIAGE RETURN (CR), DELETE LINE (DL), INSERT LINE (IL) or NEXT LINE (NEL) in the data stream; where n equals the value of Pn. It is also the position ahead of which no implicit movement of the active data position shall occur.
The established position is called the line home position and remains in effect until the next occurrence of SLH in the data stream.
8.3.123 SLL - SET LINE LIMIT
Notation: (Pn)
Representation: CSI Pn 02/00 05/06
No parameter default value.
If the DEVICE COMPONENT SELECT MODE is set to PRESENTATION, SLL is used to establish at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component the position to which the active presentation position will be moved by subsequent occurrences of CARRIAGE RETURN (CR), or NEXT LINE (NEL) in the data stream if the parameter value of SELECT IMPLICIT MOVEMENT DIRECTION (SIMD) is equal to 1; where n equals the value of Pn. In the case of a device without data component, it is also the position beyond which no implicit movement of the active presentation position shall occur.
If the DEVICE COMPONENT SELECT MODE is set to DATA, SLL is used to establish at character position n in the active line (the line that contains the active data position) and lines of subsequent text in the data component the position beyond which no implicit movement of the active data position shall occur. It is also the position in the data component to which the active data position will be moved by subsequent occurrences of CR or NEL in the data stream, if the parameter value of SELECT IMPLICIT MOVEMENT DIRECTION (SIMD) is equal to 1.
The established position is called the line limit position and remains in effect until the next occurrence of SLL in the data stream.
8.3.124 SLS - SET LINE SPACING
Notation: (Pn)
Representation: CSI Pn 02/00 06/08
No parameter default value.
SLS is used to establish the line spacing for subsequent text. The established spacing remains in effect until the next occurrence of SLS or of SELECT LINE SPACING (SVS) or of SPACING INCREMENT (SPI) in the data stream.
Pn specifies the line spacing.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.125 SM - SET MODE
Notation: (Ps...) Representation: CSI Ps... 06/08
No parameter default value.
SM causes the modes of the receiving device to be set as specified by the parameter values:
1 GUARDED AREA TRANSFER MODE (GATM)
2 KEYBOARD ACTION MODE (KAM)
3 CONTROL REPRESENTATION MODE (CRM)
4 INSERTION REPLACEMENT MODE (IRM)
5 STATUS REPORT TRANSFER MODE (SRTM)
6 ERASURE MODE (ERM)
7 LINE EDITING MODE (VEM)
8 BI-DIRECTIONAL SUPPORT MODE (BDSM)
9 DEVICE COMPONENT SELECT MODE (DCSM)
10 CHARACTER EDITING MODE (HEM)
11 POSITIONING UNIT MODE (PUM) (see F.4.1 in annex F)
12 SEND/RECEIVE MODE (SRM)
13 FORMAT EFFECTOR ACTION MODE (FEAM)
14 FORMAT EFFECTOR TRANSFER MODE (FETM)
15 MULTIPLE AREA TRANSFER MODE (MATM)
16 TRANSFER TERMINATION MODE (TTM)
17 SELECTED AREA TRANSFER MODE (SATM)
18 TABULATION STOP MODE (TSM)
19 (Shall not be used; see F.5.1 in annex F)
20 (Shall not be used; see F.5.2 in annex F)
21 GRAPHIC RENDITION COMBINATION (GRCM)
22 ZERO DEFAULT MODE (ZDM) (see F.4.2 in annex F)
NOTE
Private modes may be implemented using private parameters, see 5.4.1 and 7.4.
8.3.126 SO - SHIFT-OUT
Notation: (C0) Representation: 00/14
SO is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of SO is defined in Standard ECMA-35.
NOTE
SO is used in 7-bit environments only; in 8-bit environments LOCKING-SHIFT ONE (LS1) is used instead.
8.3.127 SOH - START OF HEADING
Notation: (C0) Representation: 00/01
SOH is used to indicate the beginning of a heading.
The use of SOH is defined in ISO 1745.
8.3.128 SOS - START OF STRING
Notation: (C1)
Representation: 09/08 or ESC 05/08
SOS is used as the opening delimiter of a control string. The character string following may consist of any bit combination, except those representing SOS or STRING TERMINATOR (ST). The control string is closed by the terminating delimiter STRING TERMINATOR (ST). The interpretation of the character string depends on the application.
8.3.129 SPA - START OF GUARDED AREA
Notation: (C1)
Representation: 09/06 or ESC 05/06
SPA is used to indicate that the active presentation position is the first of a string of character positions in the presentation component, the contents of which are protected against manual alteration, are guarded against transmission or transfer, depending on the setting of the GUARDED AREA TRANSFER MODE (GATM) and may be protected against erasure, depending on the setting of the ERASURE MODE (ERM). The end of this string is indicated by END OF GUARDED AREA (EPA).
NOTE
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string or an SDS string.
8.3.130 SPD - SELECT PRESENTATION DIRECTIONS
Notation: (Ps1;Ps2)
Representation: CSI Ps1;Ps2 02/00 05/03
Parameter default value: Ps1 = 0; Ps2 = 0
SPD is used to select the line orientation, the line progression, and the character path in the presentation component. It is also used to update the content of the presentation component and the content of the data component. This takes effect immediately.
Ps1 specifies the line orientation, the line progression and the character path:
0 line orientation: line progression: character path:
1 line orientation: line progression: character path:
2 line orientation: line progression: character path:
3 line orientation: line progression: character path:
4 line orientation: line progression: character path:
5 line orientation: line progression: character path:
horizontal top-to-bottom left-to-right
vertical right-to-left top-to-bottom
vertical left-to-right top-to-bottom
horizontal top-to-bottom right-to-left
vertical left-to-right bottom-to-top
horizontal bottom-to-top right-to-left
6 line orientation: line progression: character path:
7 line orientation: line progression: character path:
horizontal bottom-to-top left-to-right
vertical right-to-left bottom-to-top
Ps2 specifies the effect on the content of the presentation component and the content of the data component:
0 undefined (implementation-dependent)
NOTE
This may also permit the effect to take place after the next occurrence of CR, FF or any control function which initiates an absolute movement of the active presentation position or the active data position.
1 the content of the presentation component is updated to correspond to the content of the data component according to the newly established characteristics of the presentation component; the active data position is moved to the first character position in the first line in the data component, the active presentation position in the presentation component is updated accordingly
2 the content of the data component is updated to correspond to the content of the presentation component according to the newly established characteristics of the presentation component; the active presentation position is moved to the first character position in the first line in the presentation component, the active data position in the data component is updated accordingly.
8.3.131 SPH - SET PAGE HOME
Notation: (Pn)
Representation: CSI Pn 02/00 06/09
No parameter default value.
If the DEVICE COMPONENT SELECT MODE is set to PRESENTATION, SPH is used to establish at line position n in the active page (the page that contains the active presentation position) and subsequent pages in the presentation component the position to which the active presentation position will be moved by subsequent occurrences of FORM FEED (FF) in the data stream; where n equals the value of Pn. In the case of a device without data component, it is also the position ahead of which no implicit movement of the active presentation position shall occur.
If the DEVICE COMPONENT SELECT MODE is set to DATA, SPH is used to establish at line position n in the active page (the page that contains the active data position) and subsequent pages in the data component the position to which the active data position will be moved by subsequent occurrences of FORM FEED (FF) in the data stream; where n equals the value of Pn. It is also the position ahead of which no implicit movement of the active presentation position shall occur.
The established position is called the page home position and remains in effect until the next occurrence of SPH in the data stream.
8.3.132 SPI - SPACING INCREMENT
Notation: (Pn1;Pn2)
Representation: CSI Pn1;Pn2 02/00 04/07
No parameter default values.
SPI is used to establish the line spacing and the character spacing for subsequent text. The established line spacing remains in effect until the next occurrence of SPI or of SET LINE SPACING (SLS) or of SELECT LINE SPACING (SVS) in the data stream. The established character spacing remains in effect until the next occurrence of SET CHARACTER SPACING (SCS) or of SELECT CHARACTER SPACING (SHS) in the data stream, see annex C.
Pn1 specifies the line spacing
Pn2 specifies the character spacing
The unit in which the parameter values are expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.133 SPL - SET PAGE LIMIT
Notation: (Pn)
Representation: CSI Pn 02/00 06/10
No parameter default value.
If the DEVICE COMPONENT SELECT MODE is set to PRESENTATION, SPL is used to establish at line position n in the active page (the page that contains the active presentation position) and pages of subsequent text in the presentation component the position beyond which the active presentation position can normally not be moved; where n equals the value of Pn. In the case of a device without data component, it is also the position beyond which no implicit movement of the active presentation position shall occur.
If the DEVICE COMPONENT SELECT MODE is set to DATA, SPL is used to establish at line position n in the active page (the page that contains the active data position) and pages of subsequent text in the data component the position beyond which no implicit movement of the active data position shall occur.
The established position is called the page limit position and remains in effect until the next occurrence of SPL in the data stream.
8.3.134 SPQR - SELECT PRINT QUALITY AND RAPIDITY
Notation: (Ps)
Representation: CSI Ps 02/00 05/08
Parameter default value: Ps = 0
SPQR is used to select the relative print quality and the print speed for devices the output quality and speed of which are inversely related. The selected values remain in effect until the next occurrence of SPQR in the data stream. The parameter values are
0 highest available print quality, low print speed
1 medium print quality, medium print speed
2 draft print quality, highest available print speed
8.3.135 SR - SCROLL RIGHT
Notation: (Pn)
Representation: CSI Pn 02/00 04/01
Parameter default value: Pn = 1
SR causes the data in the presentation component to be moved by n character positions if the line orientation is horizontal, or by n line positions if the line orientation is vertical, such that the data appear to move to the right; where n equals the value of Pn.
The active presentation position is not affected by this control function.
8.3.136 SRCS - SET REDUCED CHARACTER SEPARATION
Notation: (Pn)
Representation: CSI Pn 02/00 06/06
Parameter default value: Pn = 0
SRCS is used to establish reduced inter-character escapement for subsequent text. The established reduced escapement remains in effect until the next occurrence of SRCS or of SET ADDITIONAL CHARACTER SEPARATION (SACS) in the data stream or until it is reset to the default value by a subsequent occurrence of CARRIAGE RETURN/LINE FEED (CR/LF) or of NEXT LINE (NEL) in the data stream, see annex C.
Pn specifies the number of units by which the inter-character escapement is reduced.
The unit in which the parameter values are expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.137 SRS - START REVERSED STRING
Notation: (Ps) Representation: CSI Ps 05/11
Parameter default value: Ps = 0
SRS is used to establish in the data component the beginning and the end of a string of characters as well as the direction of the string. This direction is opposite to that currently established. The indicated string follows the preceding text. The established character progression is not affected.
The beginning of a reversed string is indicated by SRS with a parameter value of 1. A reversed string may contain one or more nested strings. These nested strings may be reversed strings the beginnings of which are indicated by SRS with a parameter value of 1, or directed strings the beginnings of which are indicated by START DIRECTED STRING (SDS) with a parameter value not equal to 0. Every beginning of such a string invokes the next deeper level of nesting.
This Standard does not define the location of the active data position within any such nested string.
The end of a reversed string is indicated by SRS with a parameter value of 0. Every end of such a string re-establishes the next higher level of nesting (the one in effect prior to the string just ended). The direction is re-established to that in effect prior to the string just ended. The active data position is moved to the character position following the characters of the string just ended.
The parameter values are:
0 end of a reversed string; re-establish the previous direction
1 beginning of a reversed string; reverse the direction.
NOTE 1
The effect of receiving a CVT, HT, SCP, SPD or VT control function within an SRS string is not defined by this Standard.
NOTE 2
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string.
8.3.138 SSA - START OF SELECTED AREA
Notation: (C1)
Representation: 08/06 or ESC 04/06
SSA is used to indicate that the active presentation position is the first of a string of character positions in the presentation component, the contents of which are eligible to be transmitted in the form of a data stream or transferred to an auxiliary input/output device.
The end of this string is indicated by END OF SELECTED AREA (ESA). The string of characters actually transmitted or transferred depends on the setting of the GUARDED AREA TRANSFER MODE (GATM) and on any guarded areas established by DEFINE AREA QUALIFICATION (DAQ), or by START OF GUARDED AREA (SPA) and END OF GUARDED AREA (EPA).
NOTE
The control functions for area definition (DAQ, EPA, ESA, SPA, SSA) should not be used within an SRS string or an SDS string.
8.3.139 SSU - SELECT SIZE UNIT
Notation: (Ps)
Representation: CSI Ps 02/00 04/09
Parameter default value: Ps = 0
SSU is used to establish the unit in which the numeric parameters of certain control functions are expressed. The established unit remains in effect until the next occurrence of SSU in the data stream.
The parameter values are
0 CHARACTER - The dimensions of this unit are device-dependent
1 MILLIMETRE
2 COMPUTER DECIPOINT - 0,035 28 mm (1/720 of 25,4 mm)
3 DECIDIDOT - 0,037 59 mm (10/266 mm)
4 MIL-0,0254mm(1/1000of25,4mm)
5 BASIC MEASURING UNIT (BMU) - 0,021 17 mm (1/1 200 of 25,4 mm)
6 MICROMETRE - 0,001 mm
7 PIXEL - The smallest increment that can be specified in a device
8 DECIPOINT - 0,035 14 mm (35/996 mm)
8.3.140 SSW - SET SPACE WIDTH
Notation: (Pn)
Representation: CSI Pn 02/00 05/11
No parameter default value.
SSW is used to establish for subsequent text the character escapement associated with the character SPACE. The established escapement remains in effect until the next occurrence of SSW in the data stream or until it is reset to the default value by a subsequent occurrence of CARRIAGE RETURN/LINE FEED (CR/LF), CARRIAGE RETURN/FORM FEED (CR/FF), or of NEXT LINE (NEL) in the data stream, see annex C.
Pn specifies the escapement.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
The default character escapement of SPACE is specified by the most recent occurrence of SET CHARACTER SPACING (SCS) or of SELECT CHARACTER SPACING (SHS) or of SELECT SPACING INCREMENT (SPI) in the data stream if the current font has constant spacing, or is specified by the nominal width of the character SPACE in the current font if that font has proportional spacing.
8.3.141 SS2 - SINGLE-SHIFT TWO
Notation: (C1)
Representation: 08/14 or ESC 04/14
SS2 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of SS2 is defined in Standard ECMA-35.
8.3.142 SS3 - SINGLE-SHIFT THREE
Notation: (C1)
Representation: 08/15 or ESC 04/15
SS3 is used for code extension purposes. It causes the meanings of the bit combinations following it in the data stream to be changed.
The use of SS3 is defined in Standard ECMA-35.
8.3.143 ST - STRING TERMINATOR
Notation: (C1)
Representation: 09/12 or ESC 05/12
ST is used as the closing delimiter of a control string opened by APPLICATION PROGRAM COMMAND (APC), DEVICE CONTROL STRING (DCS), OPERATING SYSTEM COMMAND (OSC), PRIVACY MESSAGE (PM), or START OF STRING (SOS).
8.3.144 STAB - SELECTIVE TABULATION
Notation: (Ps)
Representation: CSI Ps 02/00 05/14
No parameter default value.
STAB causes subsequent text in the presentation component to be aligned according to the position and the properties of a tabulation stop which is selected from a list according to the value of the parameter Ps.
The use of this control function and means of specifying a list of tabulation stops to be referenced by the control function are specified in other standards, for example ISO 8613-6.
8.3.145 STS - SET TRANSMIT STATE
Notation: (C1)
Representation: 09/03 or ESC 05/03
STS is used to establish the transmit state in the receiving device. In this state the transmission of data from the device is possible.
The actual initiation of transmission of data is performed by a data communication or input/output interface control procedure which is outside the scope of this Standard.
The transmit state is established either by STS appearing in the received data stream or by the operation of an appropriate key on a keyboard.
8.3.146 STX - START OF TEXT
Notation: (C0) Representation: 00/02
STX is used to indicate the beginning of a text and the end of a heading.
The use of STX is defined in ISO 1745.
8.3.147 SU - SCROLL UP
Notation: (Pn) Representation: CSI Pn 05/03
Parameter default value: Pn = 1
SU causes the data in the presentation component to be moved by n line positions if the line orientation is horizontal, or by n character positions if the line orientation is vertical, such that the data appear to move up; where n equals the value of Pn.
The active presentation position is not affected by this control function.
8.3.148 SUB - SUBSTITUTE
Notation: (C0) Representation: 01/10
SUB is used in the place of a character that has been found to be invalid or in error. SUB is intended to be introduced by automatic means.
8.3.149 SVS - SELECT LINE SPACING
Notation: (Ps)
Representation: CSI Ps 02/00 04/12
Parameter default value: Ps = 0
SVS is used to establish the line spacing for subsequent text. The established spacing remains in effect until the next occurrence of SVS or of SET LINE SPACING (SLS) or of SPACING INCREMENT (SPI) in the data stream. The parameter values are:
0 6 lines per 25,4 mm
1 4 lines per 25,4 mm
2 3 lines per 25,4 mm
3 12 lines per 25,4 mm
4 8 lines per 25,4 mm
5 6 lines per 30,0 mm
6 4 lines per 30,0 mm
7 3 lines per 30,0 mm
8 12 lines per 30,0 mm
9 2 lines per 25,4 mm
8.3.150 SYN - SYNCHRONOUS IDLE
Notation: (C0) Representation: 01/06
SYN is used by a synchronous transmission system in the absence of any other character (idle condition) to provide a signal from which synchronism may be achieved or retained between data terminal equipment.
The use of SYN is defined in ISO 1745.
8.3.151 TAC - TABULATION ALIGNED CENTRED
Notation: (Pn)
Representation: CSI Pn 02/00 06/02
No parameter default value.
TAC causes a character tabulation stop calling for centring to be set at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component, where n equals the value of Pn. TAC causes the replacement of any tabulation stop previously set at that character position, but does not affect other tabulation stops.
A text string centred upon a tabulation stop set by TAC will be positioned so that the (trailing edge of the) first graphic character and the (leading edge of the) last graphic character are at approximately equal distances from the tabulation stop.
8.3.152 TALE - TABULATION ALIGNED LEADING EDGE
Notation: (Pn)
Representation: CSI Pn 02/00 06/01
No parameter default value.
TALE causes a character tabulation stop calling for leading edge alignment to be set at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component, where n equals the value of Pn. TALE causes the replacement of any tabulation stop previously set at that character position, but does not affect other tabulation stops.
A text string aligned with a tabulation stop set by TALE will be positioned so that the (leading edge of the) last graphic character of the string is placed at the tabulation stop.
8.3.153 TATE - TABULATION ALIGNED TRAILING EDGE
Notation: (Pn)
Representation: CSI Pn 02/00 06/00
No parameter default value.
TATE causes a character tabulation stop calling for trailing edge alignment to be set at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component, where n equals the value of Pn. TATE causes the replacement of any tabulation stop previously set at that character position, but does not affect other tabulation stops.
A text string aligned with a tabulation stop set by TATE will be positioned so that the (trailing edge of the) first graphic character of the string is placed at the tabulation stop.
8.3.154 TBC - TABULATION CLEAR
Notation: (Ps) Representation: CSI Ps 06/07
Parameter default value: Ps = 0
TBC causes one or more tabulation stops in the presentation component to be cleared, depending on the parameter value:
0 the character tabulation stop at the active presentation position is cleared
1 the line tabulation stop at the active line is cleared
2 all character tabulation stops in the active line are cleared
3 all character tabulation stops are cleared
4 all line tabulation stops are cleared
5 all tabulation stops are cleared
In the case of parameter value 0 or 2 the number of lines affected depends on the setting of the TABULATION STOP MODE (TSM)
8.3.155 TCC - TABULATION CENTRED ON CHARACTER
Notation: (Pn1;Pn2)
Representation: CSI Pn1;Pn2 02/00 06/03
No parameter default value for Pn1
Parameter default value: Pn2 = 32
TCC causes a character tabulation stop calling for alignment of a target graphic character to be set at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component, where n equals the value of Pn1, and the target character about which centring is to be performed is specified by Pn2. TCC causes the replacement of any tabulation stop previously set at that character position, but does not affect other tabulation stops.
The positioning of a text string aligned with a tabulation stop set by TCC will be determined by the first occurrence in the string of the target graphic character; that character will be centred upon the tabulation stop. If the target character does not occur within the string, then the trailing edge of the first character of the string will be positioned at the tabulation stop.
The value of Pn2 indicates the code table position (binary value) of the target character in the currently invoked code. For a 7-bit code, the permissible range of values is 32 to 127; for an 8-bit code, the permissible range of values is 32 to 127 and 160 to 255.
8.3.156 TSR - TABULATION STOP REMOVE
Notation: (Pn)
Representation: CSI Pn 02/00 06/04
No parameter default value.
TSR causes any character tabulation stop at character position n in the active line (the line that contains the active presentation position) and lines of subsequent text in the presentation component to be cleared, but does not affect other tabulation stops. n equals the value of Pn.
8.3.157 TSS - THIN SPACE SPECIFICATION
Notation: (Pn)
Representation: CSI Pn 02/00 04/05
No parameter default value.
TSS is used to establish the width of a thin space for subsequent text. The established width remains in effect until the next occurrence of TSS in the data stream, see annex C.
Pn specifies the width of the thin space.
The unit in which the parameter value is expressed is that established by the parameter value of SELECT SIZE UNIT (SSU).
8.3.158 VPA - LINE POSITION ABSOLUTE
Notation: (Pn) Representation: CSI Pn 06/04
Parameter default value: Pn = 1
VPA causes the active data position to be moved to line position n in the data component in a direction parallel to the line progression, where n equals the value of Pn.
8.3.159 VPB - LINE POSITION BACKWARD
Notation: (Pn) Representation: CSI Pn 06/11
Parameter default value: Pn = 1
VPB causes the active data position to be moved by n line positions in the data component in a direction opposite to that of the line progression, where n equals the value of Pn.
8.3.160 VPR - LINE POSITION FORWARD
Notation: (Pn) Representation: CSI Pn 06/05
Parameter default value: Pn = 1
VPR causes the active data position to be moved by n line positions in the data component in a direction parallel to the line progression, where n equals the value of Pn.
8.3.161 VT - LINE TABULATION
Notation: (C0) Representation: 00/11
VT causes the active presentation position to be moved in the presentation component to the corresponding character position on the line at which the following line tabulation stop is set.
8.3.162 VTS - LINE TABULATION SET
Notation: (C1)
Representation: 08/10 or ESC 04/10
VTS causes a line tabulation stop to be set at the active line (the line that contains the active presentation position).
8.3.xxx C0 - CONTROL SET 0 ANNOUNCER
Notation: (ESC) Representation: ESC 02/01 04/00
C0 is the 3-character escape sequence designating and invoking the C0 set.
NOTE 1
The use of this escape sequence implies that all control functions of this C0 set must be implemented.
NOTE 2
It is assumed that even with no invoked C0 set the control character ESCAPE is available and is represented by bit combination 01/11.
This sequence is described, but not named in ECMA-48.
8.3.xxx C1 - CONTROL SET 1 ANNOUNCER
Notation: (ESC) Representation: ESC 02/06 04/00
C1 is the 3-character escape sequence designating and invoking the C1 set.
NOTE:
The use of this escape sequence implies that all control characters of this C1 set must be implemented.
This sequence is described, but not named in ECMA-48.
8.3.xxx C1ALT1 - CONTROL SET 1 ANNOUNCER ALTERNATE 1
Notation: (ESC) Representation: ESC 02/00 04/06
C1ALT1, according to Standard ECMA-35, announces the control functions of the C1 set are represented by ESC Fe sequences as in a 7-bit code.
This sequence is described, but not named in ECMA-48.
8.3.xxx C1ALT2 - CONTROL SET 1 ANNOUNCER ALTERNATE 2
Notation: (ESC) Representation: ESC 02/02 04/06
C1LAT2 is an alternate 3-character escape sequence designating and invoking the C1 set.
NOTE:
The use of this escape sequence implies that all control characters of this C1 set must be implemented.
This sequence is described, but not named in ECMA-48.`, "\n8.3.")[1:]
