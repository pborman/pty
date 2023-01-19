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
	"os"
	"path"
	"reflect"
	"syscall"
	"testing"
	"time"
)

var procData = `cpu  11920047 1687761 4058579 2325550768 3007069 582 268261 0 0 0
cpu0 1238305 297363 599837 190712475 2117832 582 218698 0 0 0
cpu1 958467 199070 398008 193687372 170339 0 6786 0 0 0
intr 21 1 2 3 4 5 6
ctxt 5867995269
btime 1373498362
processes 8604517
procs_running 2
procs_blocked 3
softirq 15 1 2 3 4 5
`

var newStatTests = []struct {
	in   string
	what StatType
	out  *Stat
}{
	{procData, StatAll, &Stat{
		CPUs: []*CPU{
			&CPU{2346493067, 11920047, 1687761, 4058579, 2325550768, 3007069, 582, 268261, 0, 0, 0},
			&CPU{195185092, 1238305, 297363, 599837, 190712475, 2117832, 582, 218698, 0, 0, 0},
			&CPU{195420042, 958467, 199070, 398008, 193687372, 170339, 0, 6786, 0, 0, 0},
		},
		Interrupts:      []int64{21, 1, 2, 3, 4, 5, 6},
		ContextSwitches: 5867995269,
		BootTime:        time.Unix(1373498362, 0),
		Processes:       8604517,
		Runnable:        2,
		Blocked:         3,
		SoftInterrupts:  []int64{15, 1, 2, 3, 4, 5},
	}},
	{procData, StatCPUs, &Stat{
		CPUs: []*CPU{
			&CPU{2346493067, 11920047, 1687761, 4058579, 2325550768, 3007069, 582, 268261, 0, 0, 0},
			&CPU{195185092, 1238305, 297363, 599837, 190712475, 2117832, 582, 218698, 0, 0, 0},
			&CPU{195420042, 958467, 199070, 398008, 193687372, 170339, 0, 6786, 0, 0, 0},
		},
	}},
	{procData, StatCPU, &Stat{
		CPUs: []*CPU{
			&CPU{2346493067, 11920047, 1687761, 4058579, 2325550768, 3007069, 582, 268261, 0, 0, 0},
		},
	}},
	{procData, StatIRQ, &Stat{
		Interrupts:     []int64{21, 1, 2, 3, 4, 5, 6},
		SoftInterrupts: []int64{15, 1, 2, 3, 4, 5},
	}},
	{procData, StatProcess, &Stat{
		ContextSwitches: 5867995269,
		Processes:       8604517,
		Runnable:        2,
		Blocked:         3,
	}},
	{procData, StatBootTime, &Stat{
		BootTime: time.Unix(1373498362, 0),
	}},
}

func TestSystemStat(t *testing.T) {
	for x, tt := range newStatTests {
		out, err := NewStat(tt.what, []byte(tt.in))
		if err != nil {
			t.Errorf("#%d: unexpected error: %v", x, err)
			return
		}
		if len(out.CPUs) != len(tt.out.CPUs) {
			t.Errorf("#%d: got %d CPUs, want %d\n", x, len(out.CPUs), len(tt.out.CPUs))
			continue
		}
		for i, cpu := range out.CPUs {
			if !reflect.DeepEqual(cpu, tt.out.CPUs[i]) {
				t.Errorf("#%d: cpu%d got\n%v, want\n%v", x, i, cpu, tt.out.CPUs[i])
			}
		}
		out.CPUs = nil
		tt.out.CPUs = nil
		if !reflect.DeepEqual(out, tt.out) {
			t.Errorf("#%d: got\n%v, want\n%v", x, out, tt.out)
		}
	}
}

var cpuUsageTests = []struct {
	in                              *CPU
	user, system, idle, wait, guest float64
}{
	{
		&CPU{
			Total:     100,
			User:      1,
			Nice:      2,
			System:    3,
			Idle:      49,
			IOWait:    5,
			IRQ:       6,
			SoftIRQ:   7,
			Steal:     8,
			Guest:     9,
			GuestNice: 10,
		},
		0.03, 0.16, 0.49, 0.13, 0.19,
	},
}

func TestCpuUsage(t *testing.T) {
	for x, tt := range cpuUsageTests {
		user, system, idle, wait, guest := tt.in.Usage()
		if user != tt.user {
			t.Errorf("#%d: got user %v, want %v", x, user, tt.user)
		}
		if system != tt.system {
			t.Errorf("#%d: got system %v, want %v", x, system, tt.system)
		}
		if idle != tt.idle {
			t.Errorf("#%d: got idle %v, want %v", x, idle, tt.idle)
		}
		if wait != tt.wait {
			t.Errorf("#%d: got wait %v, want %v", x, wait, tt.wait)
		}
		if guest != tt.guest {
			t.Errorf("#%d: got guest %v, want %v", x, guest, tt.guest)
		}
	}
}

func TestProcStat(t *testing.T) {
	pid := os.Getpid()
	ps, err := ProcStat(pid)
	if err != nil {
		t.Fatal(err)
	}

	// Test a smattering of fields that we can easily verify
	cmd := path.Base(os.Args[0])
	if ps.Command != cmd {
		t.Errorf("got command %q, want %q\n", ps.Command, cmd)
	}
	if ps.Pid != pid {
		t.Errorf("got pid %d, want %d\n", ps.Pid, pid)
	}
	ppid := os.Getppid()
	if ps.PPid != ppid {
		t.Errorf("got ppid %d, want %d\n", ps.PPid, ppid)
	}
	pgid, _ := syscall.Getpgid(pid)
	if ps.PGid != pgid {
		t.Errorf("got ppid %d, want %d\n", ps.PGid, pgid)
	}

	pst, err := ProcStartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	if pst == 0 {
		t.Error("ProcStatTime is zero", pst)
	}
	if ps.StartTime != pst {
		t.Errorf("got %d, want %d\n", ps.StartTime, pst)
	}
}
