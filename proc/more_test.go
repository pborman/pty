//   Copyright 2023 Paul Borman
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package proc

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestDirectoryList(t *testing.T) {
	names, err := DirectoryList(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Error("DirectoryList(.) returned no names")
	}
	_, err = DirectoryList("this-directory-does-not-exist-pty")
	if err == nil {
		t.Error("DirectoryList(missing) succeeded")
	}
}

func TestProcessTreeManual(t *testing.T) {
	pt := &ProcessTree{
		Pids:     map[int]*Process{},
		Children: map[int][]int{},
	}
	if p := pt.Process(1); p != nil {
		t.Errorf("empty tree Process(1) = %v", p)
	}

	parent := &Process{Pid: 1, Name: "init"}
	child := &Process{Pid: 2, PPid: 1, Name: "child"}
	parent.Children = []*Process{child}
	pt.Pids[1] = parent
	pt.Pids[2] = child

	got := pt.Process(2)
	if got == nil || !got.filled {
		t.Fatalf("Process(2) = %v, want filled child", got)
	}
	// Second call should hit the filled short-circuit.
	if pt.Process(2) != got {
		t.Error("Process returned a different pointer on the second call")
	}
}

func TestFillMissingProc(t *testing.T) {
	p := &Process{Pid: -1, Name: "ghost"}
	p.Fill()
	if !p.filled {
		t.Error("Fill did not set filled")
	}
	if p.SessionID != -1 {
		t.Errorf("sessionID of missing pid = %d, want -1", p.SessionID)
	}
	if p.WD != "unknown" {
		t.Errorf("cwd of missing pid = %q, want unknown", p.WD)
	}
	if p.Argv != nil && len(p.Argv) != 0 {
		t.Logf("argv of missing pid = %q", p.Argv)
	}
	p.Fill() // already filled
}

func TestArgvCwdOpenfiles(t *testing.T) {
	if argv(-1) != nil {
		t.Errorf("argv(-1) = %q, want nil", argv(-1))
	}
	if cwd(-1) != "unknown" {
		t.Errorf("cwd(-1) = %q", cwd(-1))
	}
	if sessionID(-1) != -1 {
		t.Errorf("sessionID(-1) = %d", sessionID(-1))
	}
	files := openfiles(-1)
	if len(files) != 0 {
		t.Errorf("openfiles(-1) = %q", files)
	}
}

func TestCPUDeltaAndZeroUsage(t *testing.T) {
	zero := &CPU{}
	u, s, i, w, g := zero.Usage()
	if u != 0 || s != 0 || i != 0 || w != 0 || g != 0 {
		t.Errorf("zero Usage = %v %v %v %v %v", u, s, i, w, g)
	}

	a := &CPU{Total: 20, User: 10, System: 5, Idle: 5}
	b := &CPU{Total: 10, User: 4, System: 3, Idle: 3}
	d := a.Delta(b)
	want := &CPU{Total: 10, User: 6, System: 2, Idle: 2}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("Delta = %+v, want %+v", d, want)
	}
}

func TestNewStatOddLines(t *testing.T) {
	data := []byte("nospace\ncpu  1 2 3 4 5 6 7 8 9 10\ncpu\n")
	st, err := NewStat(StatCPU, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CPUs) != 1 {
		t.Errorf("got %d CPUs, want 1", len(st.CPUs))
	}

	st, err = NewStat(0, []byte("cpu  1 2 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CPUs) != 0 {
		t.Errorf("what=0 still parsed CPUs: %+v", st.CPUs)
	}
}

func TestSysconf(t *testing.T) {
	n := Sysconf(SCClkTck)
	if n <= 0 {
		t.Errorf("Sysconf(SCClkTck) = %d, want > 0", n)
	}
}

func TestGoNameMore(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"$$$", ""},
		{"123", "X123"},
		{"βeta", "Xβeta"},
		{"foo-bar-baz", "FooBarBaz"},
	} {
		if got := GoName(tt.in); got != tt.want {
			t.Errorf("GoName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseProcFileScale(t *testing.T) {
	out := &struct{ Field int }{}
	if err := ParseProcFile(bytes.NewBufferString("Field: 2 mB\n"), out); err != nil {
		t.Fatal(err)
	}
	if out.Field != 2*1024*1024 {
		t.Errorf("mB scale: got %d, want %d", out.Field, 2*1024*1024)
	}

	out = &struct{ Field int }{}
	if err := ParseProcFile(bytes.NewBufferString("Field: 3 kB"), out); err != nil {
		t.Fatal(err)
	}
	if out.Field != 3*1024 {
		t.Errorf("kB scale without newline: got %d", out.Field)
	}

	out = &struct{ Field int }{}
	if err := ParseProcFile(bytes.NewBufferString("Unknown: 1\nField: 7\n"), out); err != nil {
		t.Fatal(err)
	}
	if out.Field != 7 {
		t.Errorf("skipped unknown field: got %d", out.Field)
	}
}

func TestProcHelpersWithoutProc(t *testing.T) {
	if _, err := os.Stat("/proc"); err == nil {
		t.Skip("/proc exists; error-path coverage is for other systems")
	}
	if _, err := NewProcessTree(); err == nil {
		t.Error("NewProcessTree succeeded without /proc")
	}
	if _, err := PS(1); err == nil {
		t.Error("PS succeeded without /proc")
	}
	if _, err := Me(); err == nil {
		t.Error("Me succeeded without /proc")
	}
	if _, err := GetStatus(1); err == nil {
		t.Error("GetStatus succeeded without /proc")
	}
	if _, err := SystemStat(StatAll); err == nil {
		t.Error("SystemStat succeeded without /proc")
	}
	if _, err := ProcStat(1); err == nil {
		t.Error("ProcStat succeeded without /proc")
	}
	if _, err := ProcStartTime(1); err == nil {
		t.Error("ProcStartTime succeeded without /proc")
	}
}

func TestProcHelpersWithProc(t *testing.T) {
	if _, err := os.Stat("/proc"); err != nil {
		t.Skip("/proc not available")
	}
	st, err := SystemStat(StatCPU)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CPUs) == 0 {
		t.Error("SystemStat(StatCPU) returned no CPUs")
	}
	me, err := Me()
	if err != nil {
		t.Fatal(err)
	}
	if me.Pid != os.Getpid() {
		t.Errorf("Me pid = %d, want %d", me.Pid, os.Getpid())
	}
	if _, err := GetStatus(os.Getpid()); err != nil {
		t.Fatal(err)
	}
}
