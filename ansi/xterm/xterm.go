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
	return fmt.Errorf("duplicated codes: %q", dups)
}

// Mode "C1 (8-Bit) Control Characters"

var IND_ = ansi.Sequence{
	Name: "IND",
	Code: []byte{ESC, 'D'},
}

// Mode "VT100 Mode" Controls beginning with ESC

var S8C1T_ = ansi.Sequence{
	Name: "S8C1T",
	Code: []byte{ESC, ' ', 'G'},
}

var SANC1_ = ansi.Sequence{
	Name: "SANC1",
	Code: []byte{ESC, ' ', 'L'},
}

var SANC2_ = ansi.Sequence{
	Name: "SANC2",
	Code: []byte{ESC, ' ', 'M'},
}

var SANC3_ = ansi.Sequence{
	Name: "SANC3",
	Code: []byte{ESC, ' ', 'N'},
}

var DECDHLT_ = ansi.Sequence{
	Name: "DECDHLT",
	Code: []byte{ESC, '#', '3'},
}

var DECDHLB_ = ansi.Sequence{
	Name: "DECDHLB",
	Code: []byte{ESC, '#', '4'},
}

var DECSWL_ = ansi.Sequence{
	Name: "DECSWL",
	Code: []byte{ESC, '#', '5'},
}

var DECDWL_ = ansi.Sequence{
	Name: "DECDWL",
	Code: []byte{ESC, '#', '6'},
}

var DECALN_ = ansi.Sequence{
	Name: "DECALN",
	Code: []byte{ESC, '#', '8'},
}

var SDCS_ = ansi.Sequence{
	Name: "SDCS",
	Code: []byte{ESC, '%', '@'},
}

var SUTF8_ = ansi.Sequence{
	Name: "SUTF8",
	Code: []byte{ESC, '%', 'G'},
}

var DESG0CS_ = ansi.Sequence{
	Name: "DESG0CS",
	Code: []byte{ESC, '('},
}

var DESG1CS_ = ansi.Sequence{
	Name: "DESG1CS",
	Code: []byte{ESC, ')'},
}

var DESG2CS_ = ansi.Sequence{
	Name: "DESG2CS",
	Code: []byte{ESC, '*'},
}

var DESG3CS_ = ansi.Sequence{
	Name: "DESG3CS",
	Code: []byte{ESC, '+'},
}

var DESG1CSVT300_ = ansi.Sequence{
	Name: "DESG1CSVT300",
	Code: []byte{ESC, '-'},
}

var DESG2CSVT300_ = ansi.Sequence{
	Name: "DESG2CSVT300",
	Code: []byte{ESC, '.'},
}

var DESG3CSVT300_ = ansi.Sequence{
	Name: "DESG3CSVT300",
	Code: []byte{ESC, '/'},
}

var DECBI_ = ansi.Sequence{
	Name: "DECBI",
	Code: []byte{ESC, '6'},
}

var DECSC_ = ansi.Sequence{
	Name: "DECSC",
	Code: []byte{ESC, '7'},
}

var DECRC_ = ansi.Sequence{
	Name: "DECRC",
	Code: []byte{ESC, '8'},
}

var DECFI_ = ansi.Sequence{
	Name: "DECFI",
	Code: []byte{ESC, '9'},
}

var DECKPAM_ = ansi.Sequence{
	Name: "DECKPAM",
	Code: []byte{ESC, '='},
}

var DECKPNM_ = ansi.Sequence{
	Name: "DECKPNM",
	Code: []byte{ESC, '>'},
}

var HPMEMLOCK_ = ansi.Sequence{
	Name: "HPMEMLOCK",
	Code: []byte{ESC, 'l'},
}

var HPMEMUNLOCK_ = ansi.Sequence{
	Name: "HPMEMUNLOCK",
	Code: []byte{ESC, 'm'},
}

// Mode "VT100 Mode" Device-Control functions

var DECRQSS_ = ansi.Sequence{
	Name: "DECRQSS",
	Code: []byte{ESC, 'P', '$', 'q'},
}

var SETTI_ = ansi.Sequence{
	Name: "SETTI",
	Code: []byte{ESC, 'P', '+', 'p'},
}

var REQTI_ = ansi.Sequence{
	Name: "REQTI",
	Code: []byte{ESC, 'P', '+', 'q'},
}

// Mode "VT100 Mode" Functions using CSI, ordered by the final character(s)

var DECSED_ = ansi.Sequence{
	Name: "DECSED",
	Code: []byte{ESC, '[', '?', 'J'},
}

var DECSEL_ = ansi.Sequence{
	Name: "DECSEL",
	Code: []byte{ESC, '[', '?', 'K'},
}

var SUSIXEL_ = ansi.Sequence{
	Name: "SUSIXEL",
	Code: []byte{ESC, '[', '?', 'S'},
}

var RESETTITLE_ = ansi.Sequence{
	Name: "RESETTITLE",
	Code: []byte{ESC, '[', '>', 'T'},
}

var SENDDA_ = ansi.Sequence{
	Name: "SENDDA",
	Code: []byte{ESC, '[', '>', 'c'},
}

var DECSET_ = ansi.Sequence{
	Name: "DECSET",
	Code: []byte{ESC, '[', '?', 'h'},
}

var DECMC_ = ansi.Sequence{
	Name: "DECMC",
	Code: []byte{ESC, '[', '?', 'i'},
}

var DECRST_ = ansi.Sequence{
	Name: "DECRST",
	Code: []byte{ESC, '[', '?', 'l'},
}

var DECSETMOD_ = ansi.Sequence{
	Name: "DECSETMOD",
	Code: []byte{ESC, '[', '>', 'm'},
}

var DECDISMOD_ = ansi.Sequence{
	Name: "DECDISMOD",
	Code: []byte{ESC, '[', '>', 'n'},
}

var DSR_ = ansi.Sequence{
	Name: "DSR",
	Code: []byte{ESC, '[', '?', 'n'},
}

var SETPRINT_ = ansi.Sequence{
	Name: "SETPRINT",
	Code: []byte{ESC, '[', '>', 'p'},
}

var DECSTR_ = ansi.Sequence{
	Name: "DECSTR",
	Code: []byte{ESC, '[', '!', 'p'},
}

var DECSCL_ = ansi.Sequence{
	Name: "DECSCL",
	Code: []byte{ESC, '[', '"', 'p'},
}

var DECRQMANSI_ = ansi.Sequence{
	Name: "DECRQMANSI",
	Code: []byte{ESC, '[', '$', 'p'},
}

var DECRQMPRIVATE_ = ansi.Sequence{
	Name: "DECRQMPRIVATE",
	Code: []byte{ESC, '[', '?', '$', 'p'},
}

var DECLL_ = ansi.Sequence{
	Name: "DECLL",
	Code: []byte{ESC, '[', 'q'},
}

var DECSCUSR_ = ansi.Sequence{
	Name: "DECSCUSR",
	Code: []byte{ESC, '[', ' ', 'q'},
}

var DECSCA_ = ansi.Sequence{
	Name: "DECSCA",
	Code: []byte{ESC, '[', '"', 'q'},
}

var DECSTBM_ = ansi.Sequence{
	Name: "DECSTBM",
	Code: []byte{ESC, '[', 'r'},
}

var DECREST_ = ansi.Sequence{
	Name: "DECREST",
	Code: []byte{ESC, '[', '?', 'r'},
}

var DECCARA_ = ansi.Sequence{
	Name: "DECCARA",
	Code: []byte{ESC, '[', '$', 'r'},
}

var SCOSC_ = ansi.Sequence{
	Name: "SCOSC",
	Code: []byte{ESC, '[', 's'},
}

var DECSAVMOD_ = ansi.Sequence{
	Name: "DECSAVMOD",
	Code: []byte{ESC, '[', '?', 's'},
}

var WINMOD_ = ansi.Sequence{
	Name: "WINMOD",
	Code: []byte{ESC, '[', 't'},
}

var SETTITLE_ = ansi.Sequence{
	Name: "SETTITLE",
	Code: []byte{ESC, '[', '>', 't'},
}

var DECSWBV_ = ansi.Sequence{
	Name: "DECSWBV",
	Code: []byte{ESC, '[', ' ', 't'},
}

var DECRARA_ = ansi.Sequence{
	Name: "DECRARA",
	Code: []byte{ESC, '[', '$', 't'},
}

var SCORC_ = ansi.Sequence{
	Name: "SCORC",
	Code: []byte{ESC, '[', 'u'},
}

var DECSMBV_ = ansi.Sequence{
	Name: "DECSMBV",
	Code: []byte{ESC, '[', ' ', 'u'},
}

var DECCRA_ = ansi.Sequence{
	Name: "DECCRA",
	Code: []byte{ESC, '[', '$', 'v'},
}

var DECEFR_ = ansi.Sequence{
	Name: "DECEFR",
	Code: []byte{ESC, '[', '`', 'w'},
}

var DECREQTPARM_ = ansi.Sequence{
	Name: "DECREQTPARM",
	Code: []byte{ESC, '[', 'x'},
}

var DECSACE_ = ansi.Sequence{
	Name: "DECSACE",
	Code: []byte{ESC, '[', '*', 'x'},
}

var DECFRA_ = ansi.Sequence{
	Name: "DECFRA",
	Code: []byte{ESC, '[', '$', 'x'},
}

var DECRQCRA_ = ansi.Sequence{
	Name: "DECRQCRA",
	Code: []byte{ESC, '[', '*', 'y'},
}

var DECELR_ = ansi.Sequence{
	Name: "DECELR",
	Code: []byte{ESC, '[', '`', 'z'},
}

var DECERA_ = ansi.Sequence{
	Name: "DECERA",
	Code: []byte{ESC, '[', '$', 'z'},
}

var DECSLE_ = ansi.Sequence{
	Name: "DECSLE",
	Code: []byte{ESC, '[', '`', '{'},
}

var DECSERA_ = ansi.Sequence{
	Name: "DECSERA",
	Code: []byte{ESC, '[', '$', '{'},
}

var DECRQLP_ = ansi.Sequence{
	Name: "DECRQLP",
	Code: []byte{ESC, '[', '`', '|'},
}

var DECIC_ = ansi.Sequence{
	Name: "DECIC",
	Code: []byte{ESC, '[', '`', '}'},
}

var DECDC_ = ansi.Sequence{
	Name: "DECDC",
	Code: []byte{ESC, '[', '`', '~'},
}

const (
	IND           = ansi.Name("\033D")
	S8C1T         = ansi.Name("\033 G")
	SANC1         = ansi.Name("\033 L")
	SANC2         = ansi.Name("\033 M")
	SANC3         = ansi.Name("\033 N")
	DECDHLT       = ansi.Name("\033#3")
	DECDHLB       = ansi.Name("\033#4")
	DECSWL        = ansi.Name("\033#5")
	DECDWL        = ansi.Name("\033#6")
	DECALN        = ansi.Name("\033#8")
	SDCS          = ansi.Name("\033%@")
	SUTF8         = ansi.Name("\033%G")
	DESG0CS       = ansi.Name("\033(")
	DESG1CS       = ansi.Name("\033)")
	DESG2CS       = ansi.Name("\033*")
	DESG3CS       = ansi.Name("\033+")
	DESG1CSVT300  = ansi.Name("\033-")
	DESG2CSVT300  = ansi.Name("\033.")
	DESG3CSVT300  = ansi.Name("\033/")
	DECBI         = ansi.Name("\0336")
	DECSC         = ansi.Name("\0337")
	DECRC         = ansi.Name("\0338")
	DECFI         = ansi.Name("\0339")
	DECKPAM       = ansi.Name("\033=")
	DECKPNM       = ansi.Name("\033>")
	HPMEMLOCK     = ansi.Name("\033l")
	HPMEMUNLOCK   = ansi.Name("\033m")
	DECRQSS       = ansi.Name("\033P$q")
	SETTI         = ansi.Name("\033P+p")
	REQTI         = ansi.Name("\033P+q")
	DECSED        = ansi.Name("\033[?J")
	DECSEL        = ansi.Name("\033[?K")
	SUSIXEL       = ansi.Name("\033[?S")
	RESETTITLE    = ansi.Name("\033[>T")
	SENDDA        = ansi.Name("\033[>c")
	DECSET        = ansi.Name("\033[?h")
	DECMC         = ansi.Name("\033[?i")
	DECRST        = ansi.Name("\033[?l")
	DECSETMOD     = ansi.Name("\033[>m")
	DECDISMOD     = ansi.Name("\033[>n")
	DSR           = ansi.Name("\033[?n")
	SETPRINT      = ansi.Name("\033[>p")
	DECSTR        = ansi.Name("\033[!p")
	DECSCL        = ansi.Name("\033[\"p")
	DECRQMANSI    = ansi.Name("\033[$p")
	DECRQMPRIVATE = ansi.Name("\033[?$p")
	DECLL         = ansi.Name("\033[q")
	DECSCUSR      = ansi.Name("\033[ q")
	DECSCA        = ansi.Name("\033[\"q")
	DECSTBM       = ansi.Name("\033[r")
	DECREST       = ansi.Name("\033[?r")
	DECCARA       = ansi.Name("\033[$r")
	SCOSC         = ansi.Name("\033[s")
	DECSAVMOD     = ansi.Name("\033[?s")
	WINMOD        = ansi.Name("\033[t")
	SETTITLE      = ansi.Name("\033[>t")
	DECSWBV       = ansi.Name("\033[ t")
	DECRARA       = ansi.Name("\033[$t")
	SCORC         = ansi.Name("\033[u")
	DECSMBV       = ansi.Name("\033[ u")
	DECCRA        = ansi.Name("\033[$v")
	DECEFR        = ansi.Name("\033[`w")
	DECREQTPARM   = ansi.Name("\033[x")
	DECSACE       = ansi.Name("\033[*x")
	DECFRA        = ansi.Name("\033[$x")
	DECRQCRA      = ansi.Name("\033[*y")
	DECELR        = ansi.Name("\033[`z")
	DECERA        = ansi.Name("\033[$z")
	DECSLE        = ansi.Name("\033[`{")
	DECSERA       = ansi.Name("\033[${")
	DECRQLP       = ansi.Name("\033[`|")
	DECIC         = ansi.Name("\033[`}")
	DECDC         = ansi.Name("\033[`~")
)

var Table = map[ansi.Name]*ansi.Sequence{
	IND:           &IND_,
	S8C1T:         &S8C1T_,
	SANC1:         &SANC1_,
	SANC2:         &SANC2_,
	SANC3:         &SANC3_,
	DECDHLT:       &DECDHLT_,
	DECDHLB:       &DECDHLB_,
	DECSWL:        &DECSWL_,
	DECDWL:        &DECDWL_,
	DECALN:        &DECALN_,
	SDCS:          &SDCS_,
	SUTF8:         &SUTF8_,
	DESG0CS:       &DESG0CS_,
	DESG1CS:       &DESG1CS_,
	DESG2CS:       &DESG2CS_,
	DESG3CS:       &DESG3CS_,
	DESG1CSVT300:  &DESG1CSVT300_,
	DESG2CSVT300:  &DESG2CSVT300_,
	DESG3CSVT300:  &DESG3CSVT300_,
	DECBI:         &DECBI_,
	DECSC:         &DECSC_,
	DECRC:         &DECRC_,
	DECFI:         &DECFI_,
	DECKPAM:       &DECKPAM_,
	DECKPNM:       &DECKPNM_,
	HPMEMLOCK:     &HPMEMLOCK_,
	HPMEMUNLOCK:   &HPMEMUNLOCK_,
	DECRQSS:       &DECRQSS_,
	SETTI:         &SETTI_,
	REQTI:         &REQTI_,
	DECSED:        &DECSED_,
	DECSEL:        &DECSEL_,
	SUSIXEL:       &SUSIXEL_,
	RESETTITLE:    &RESETTITLE_,
	SENDDA:        &SENDDA_,
	DECSET:        &DECSET_,
	DECMC:         &DECMC_,
	DECRST:        &DECRST_,
	DECSETMOD:     &DECSETMOD_,
	DECDISMOD:     &DECDISMOD_,
	DSR:           &DSR_,
	SETPRINT:      &SETPRINT_,
	DECSTR:        &DECSTR_,
	DECSCL:        &DECSCL_,
	DECRQMANSI:    &DECRQMANSI_,
	DECRQMPRIVATE: &DECRQMPRIVATE_,
	DECLL:         &DECLL_,
	DECSCUSR:      &DECSCUSR_,
	DECSCA:        &DECSCA_,
	DECSTBM:       &DECSTBM_,
	DECREST:       &DECREST_,
	DECCARA:       &DECCARA_,
	SCOSC:         &SCOSC_,
	DECSAVMOD:     &DECSAVMOD_,
	WINMOD:        &WINMOD_,
	SETTITLE:      &SETTITLE_,
	DECSWBV:       &DECSWBV_,
	DECRARA:       &DECRARA_,
	SCORC:         &SCORC_,
	DECSMBV:       &DECSMBV_,
	DECCRA:        &DECCRA_,
	DECEFR:        &DECEFR_,
	DECREQTPARM:   &DECREQTPARM_,
	DECSACE:       &DECSACE_,
	DECFRA:        &DECFRA_,
	DECRQCRA:      &DECRQCRA_,
	DECELR:        &DECELR_,
	DECERA:        &DECERA_,
	DECSLE:        &DECSLE_,
	DECSERA:       &DECSERA_,
	DECRQLP:       &DECRQLP_,
	DECIC:         &DECIC_,
	DECDC:         &DECDC_,
}
