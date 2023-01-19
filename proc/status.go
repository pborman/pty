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
	"strconv"
)

type Status struct {
	Name                     string   // Name
	State                    string   // State
	Tgid                     int64    // Tgid
	Pid                      int64    // Pid
	PPid                     int64    // PPid
	TracerPid                int64    // TracerPid
	Uid                      []int    // Uid
	Gid                      []int    // Gid
	FDSize                   uint64   // FDSize
	Groups                   []int    // Groups
	VmPeak                   uint64   // VmPeak
	VmSize                   uint64   // VmSize
	VmLck                    uint64   // VmLck
	VmHWM                    uint64   // VmHWM
	VmRSS                    uint64   // VmRSS
	VmData                   uint64   // VmData
	VmStk                    uint64   // VmStk
	VmExe                    uint64   // VmExe
	VmLib                    uint64   // VmLib
	VmPTE                    uint64   // VmPTE
	VmSwap                   uint64   // VmSwap
	Threads                  uint64   // Threads
	SigQ                     []uint64 `delim:"/"` // SigQ
	SigPnd                   uint64   `base:"16"` // SigPnd
	ShdPnd                   uint64   `base:"16"` // ShdPnd
	SigBlk                   uint64   `base:"16"` // SigBlk
	SigIgn                   uint64   `base:"16"` // SigIgn
	SigCgt                   uint64   `base:"16"` // SigCgt
	CapInh                   uint64   `base:"16"` // CapInh
	CapPrm                   uint64   `base:"16"` // CapPrm
	CapEff                   uint64   `base:"16"` // CapEff
	CapBnd                   uint64   `base:"16"` // CapBnd
	CpusAllowed              []uint32 `base:"16"` // Cpus_allowed
	MemsAllowed              []uint32 `base:"16"` // Mems_allowed
	VoluntaryCtxtSwitches    uint64   // voluntary_ctxt_switches
	NonvoluntaryCtxtSwitches uint64   // nonvoluntary_ctxt_switches
}

// GetStatus returns a reference to a Status that has been filled in
// from the current values found in /proc/<pid>/status.
func GetStatus(pid int) (*Status, error) {
	f, err := os.Open("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	status := &Status{}
	if err := ParseProcFile(f, status); err != nil {
		return nil, err
	}
	return status, nil
}
