// Copyright 2012 Google Inc. All rights reserved.
// Author: borman@google.com (Paul Borman)

package proc

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

var parseTests = []struct {
	name string
	in   string
	out  interface{}
	err  string
}{
	{
		name: "colon",
		in:   `Field: 100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "space",
		in:   `Field 100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "tab",
		in:   "Field\t100",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "trailing space",
		in:   `Field: 100 `,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "space-colon-space",
		in:   `Field : 100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "space-space",
		in:   `Field  100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "colon-space-space",
		in:   `Field:  100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "tab-colon-tab",
		in:   "Field\t:\t100",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "colon-tab",
		in:   "Field:\t100",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "trailing-tab",
		in:   "Field:\t100\t",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "tab-tab",
		in:   "Field\t\t100",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "surrounding-tab-tab",
		in:   "Field\t\t100\t\t",
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "colon-tab-tab",
		in:   "Field:\t\t100",
		out:  &struct{ Field string }{"100"},
	},

	{
		name: "int",
		in:   `Field: 100`,
		out:  &struct{ Field int }{100},
	},
	{
		name: "rune",
		in:   `Field: 100`,
		out:  &struct{ Field rune }{100},
	},
	{
		name: "byte",
		in:   `Field: 100`,
		out:  &struct{ Field byte }{100},
	},
	{
		name: "int8",
		in:   `Field: 100`,
		out:  &struct{ Field int8 }{100},
	},
	{
		name: "int16",
		in:   `Field: 100`,
		out:  &struct{ Field int16 }{100},
	},
	{
		name: "int32",
		in:   `Field: 100`,
		out:  &struct{ Field int32 }{100},
	},
	{
		name: "int64",
		in:   `Field: 100`,
		out:  &struct{ Field int64 }{100},
	},
	{
		name: "uint",
		in:   `Field: 100`,
		out:  &struct{ Field uint }{100},
	},
	{
		name: "uint8",
		in:   `Field: 100`,
		out:  &struct{ Field uint8 }{100},
	},
	{
		name: "uint16",
		in:   `Field: 100`,
		out:  &struct{ Field uint16 }{100},
	},
	{
		name: "uint32",
		in:   `Field: 100`,
		out:  &struct{ Field uint32 }{100},
	},
	{
		name: "uint64",
		in:   `Field: 100`,
		out:  &struct{ Field uint64 }{100},
	},
	{
		name: "base16",
		in:   `Field: 100`,
		out: &struct {
			Field int `base:"16"`
		}{0x100},
	},
	{
		name: "base8",
		in:   `Field: 100`,
		out: &struct {
			Field int `base:"8"`
		}{0100},
	},
	{
		name: "base2",
		in:   `Field: 100`,
		out: &struct {
			Field int `base:"2"`
		}{4},
	},
	{
		name: "string",
		in:   `Field: 100`,
		out:  &struct{ Field string }{"100"},
	},
	{
		name: "float32",
		in:   `Field: 100`,
		out:  &struct{ Field float32 }{100},
	},
	{
		name: "float64",
		in:   `Field: 100`,
		out:  &struct{ Field float64 }{100},
	},
	{
		name: "[]int",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int }{[]int{10, 11, 12}},
	},
	{
		name: "[]comma",
		in:   `Field: 10,11,12`,
		out:  &struct{ Field []int }{[]int{10, 11, 12}},
	},
	{
		name: "[]slash",
		in:   `Field: 10/11/12`,
		out: &struct {
			Field []int `delim:"/"`
		}{[]int{10, 11, 12}},
	},
	{
		name: "[]base16",
		in:   `Field: 10 11 12`,
		out: &struct {
			Field []int `base:"16"`
		}{[]int{0x10, 0x11, 0x12}},
	},
	{
		name: "[]byte",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []byte }{[]byte{10, 11, 12}},
	},
	{
		name: "[]uint8",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []uint8 }{[]uint8{10, 11, 12}},
	},
	{
		name: "[]uint16",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []uint16 }{[]uint16{10, 11, 12}},
	},
	{
		name: "[]uint32",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []uint32 }{[]uint32{10, 11, 12}},
	},
	{
		name: "[]uint64",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []uint64 }{[]uint64{10, 11, 12}},
	},
	{
		name: "[]rune",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []rune }{[]rune{10, 11, 12}},
	},
	{
		name: "[]int",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int }{[]int{10, 11, 12}},
	},
	{
		name: "[]int8",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int8 }{[]int8{10, 11, 12}},
	},
	{
		name: "[]int16",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int16 }{[]int16{10, 11, 12}},
	},
	{
		name: "[]int32",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int32 }{[]int32{10, 11, 12}},
	},
	{
		name: "[]int64",
		in:   `Field: 10 11 12`,
		out:  &struct{ Field []int64 }{[]int64{10, 11, 12}},
	},
	{
		name: "[]float32",
		in:   `Field: 10.5 11.5 12.5`,
		out:  &struct{ Field []float32 }{[]float32{10.5, 11.5, 12.5}},
	},
	{
		name: "[]float64",
		in:   `Field: 10.5 11.5 12.5`,
		out:  &struct{ Field []float64 }{[]float64{10.5, 11.5, 12.5}},
	},

	{
		name: "bad-number",
		in:   `Field: duck`,
		out:  &struct{ Field int }{},
		err:  `"duck": invalid syntax`,
	},
	{
		name: "bad-slice",
		in:   `Field: duck`,
		out:  &struct{ Field []int }{},
		err:  `"duck": invalid syntax`,
	},
	{
		name: "bad-slice2",
		in:   `Field: 1 duck`,
		out:  &struct{ Field []int }{},
		err:  `"duck": invalid syntax`,
	},
	{
		name: "range256",
		in:   `Field: 256`,
		out:  &struct{ Field uint8 }{},
		err:  `"256": value out of range`,
	},
	{
		name: "range128",
		in:   `Field: 128`,
		out:  &struct{ Field int8 }{},
		err:  `"128": value out of range`,
	},
	{
		name: "range-1",
		in:   `Field: -1`,
		out:  &struct{ Field uint8 }{},
		err:  `"-1": invalid syntax`,
	},
	{
		name: "base0",
		in:   `Field: 1`,
		out: &struct {
			Field uint8 `base:"0"`
		}{},
		err: `invalid base: 0`,
	},
	{
		name: "base37",
		in:   `Field: 1`,
		out: &struct {
			Field uint8 `base:"37"`
		}{},
		err: `invalid base: 37`,
	},
	{
		name: "base36",
		in:   `Field: 10`,
		out: &struct {
			Field uint8 `base:"36"`
		}{36},
	},
	{
		name: "Z",
		in:   `Field: Z`,
		out: &struct {
			Field uint8 `base:"36"`
		}{35},
	},
	{
		name: "status",
		in: `Name:	ksh
State:	S (sleeping)
Tgid:	27920
Pid:	27920
PPid:	27919
TracerPid:	0
Uid:	116105	116105	116105	116105
Gid:	5000	5000	5000	5000
FDSize:	256
Groups:	4 24 104 499 5000 5001 5762 6833 73488 74366 75192 75209 76026 76361 77056 77281 
VmPeak:	   10636 kB
VmSize:	   10636 kB
VmLck:	       0 kB
VmHWM:	    2068 kB
VmRSS:	    2068 kB
VmData:	     872 kB
VmStk:	     136 kB
VmExe:	    1212 kB
VmLib:	    2168 kB
VmPTE:	      44 kB
VmSwap:	       0 kB
Threads:	1
SigQ:	23/16382
SigPnd:	0000000000000000
ShdPnd:	0000000000000000
SigBlk:	0000000000000000
SigIgn:	0000000000300000
SigCgt:	000000007fcb7aff
CapInh:	0000000000000000
CapPrm:	0000000000000000
CapEff:	0000000000000000
CapBnd:	ffffffffffffffff
Cpus_allowed:	ffffffff
Cpus_allowed_list:	0-31
Mems_allowed:	00000000,00000001
Mems_allowed_list:	0
voluntary_ctxt_switches:	334
nonvoluntary_ctxt_switches:	6
`,
		out: &Status{
			Name:                     "ksh",
			State:                    "S (sleeping)",
			Tgid:                     27920,
			Pid:                      27920,
			PPid:                     27919,
			TracerPid:                0,
			Uid:                      []int{116105, 116105, 116105, 116105},
			Gid:                      []int{5000, 5000, 5000, 5000},
			FDSize:                   256,
			Groups:                   []int{4, 24, 104, 499, 5000, 5001, 5762, 6833, 73488, 74366, 75192, 75209, 76026, 76361, 77056, 77281},
			VmPeak:                   10636 << 10,
			VmSize:                   10636 << 10,
			VmLck:                    0 << 10,
			VmHWM:                    2068 << 10,
			VmRSS:                    2068 << 10,
			VmData:                   872 << 10,
			VmStk:                    136 << 10,
			VmExe:                    1212 << 10,
			VmLib:                    2168 << 10,
			VmPTE:                    44 << 10,
			VmSwap:                   0 << 10,
			Threads:                  1,
			SigQ:                     []uint64{23, 16382},
			SigPnd:                   0x0000000000000000,
			ShdPnd:                   0x0000000000000000,
			SigBlk:                   0x0000000000000000,
			SigIgn:                   0x0000000000300000,
			SigCgt:                   0x000000007fcb7aff,
			CapInh:                   0x0000000000000000,
			CapPrm:                   0x0000000000000000,
			CapEff:                   0x0000000000000000,
			CapBnd:                   0xffffffffffffffff,
			CpusAllowed:              []uint32{0xffffffff},
			MemsAllowed:              []uint32{0x00000000, 0x00000001},
			VoluntaryCtxtSwitches:    334,
			NonvoluntaryCtxtSwitches: 6,
		},
	},
}

func TestParseProcFile(t *testing.T) {
	for x, tt := range parseTests {
		if tt.name == "" {
			tt.name = "#" + strconv.Itoa(x)
		}
		o := reflect.New(reflect.TypeOf(tt.out).Elem()).Interface()
		err := ParseProcFile(bytes.NewBufferString(tt.in), o)
		switch {
		case err == nil && tt.err == "":
			if !reflect.DeepEqual(o, tt.out) {
				t.Errorf("%s: Got %v, want %v", tt.name, o, tt.out)
			}
		case err != nil && tt.err == "":
			t.Errorf("%s: Unexpected error: %v", tt.name, err)
		case err == nil && tt.err != "":
			t.Errorf("%s: Did not get expected error: %v", tt.name, tt.err)
		case !strings.Contains(err.Error(), tt.err):
			t.Errorf("%s: got error %v, want %v", tt.name, err, tt.err)
		}
	}
}
