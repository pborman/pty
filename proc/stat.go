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
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

// TickDuration is the duration of a single system tick
var TickDuration = time.Second / time.Duration(Sysconf(SCClkTck))

// A CPU contains the information of a singe cpu line from /proc/stat.
// All values are in "jiffies".
type CPU struct {
	Total     int64 // Total number of jiffies
	User      int64 // normal processes executing in user mode
	Nice      int64 // niced processes executing in user mode
	System    int64 // processes executing in kernel mode
	Idle      int64 // twiddling thumbs
	IOWait    int64 // waiting for I/O to complete
	IRQ       int64 // servicing interrupts
	SoftIRQ   int64 // servicing softirqs
	Steal     int64 // involuntary wait
	Guest     int64 // running a normal guest
	GuestNice int64 // running a niced guest
}

// A Stat contains the information from /proc/stat.
type Stat struct {
	BootTime        time.Time
	CPUs            []*CPU
	ContextSwitches int64
	Processes       int64
	Runnable        int64
	Blocked         int64
	Interrupts      []int64
	SoftInterrupts  []int64
}

// A StatType is a bitfield of types of information that SystemStat gathers.
type StatType int64

const (
	StatAll       = ^StatType(0)           // All information
	StatCPU       = StatType(1 << iota)    // Only the first CPU line (the sum)
	StatCPUs                               // All CPU lines
	StatIRQ                                // Interrupts and SoftInterrupts
	StatProcess                            // ContextSwitches, Processes, Runnable, Blocked
	StatBootTime                           // BootTime
	maxRetries    = 5                      // Maximum number to times to retry file reads.
	retryInterval = 500 * time.Millisecond // Interval to wait between file open attempts.

)

// SystemStat returns the parsed contents of the /proc/stat.  The what argument
// determines what information is parsed.  An error is returned if there was an
// error reading /proc/stat.  Unrecognized data in /proc/stat is ignored.
func SystemStat(what StatType) (*Stat, error) {
	var lastErr error
	for i := 1; ; i++ {
		data, err := ioutil.ReadFile("/proc/stat")
		if err == nil {
			return NewStat(what, data)
		}
		lastErr = err
		if i == maxRetries {
			break
		}
		time.Sleep(retryInterval)
	}
	return nil, lastErr
}

// NewStat returns a new instance of Stat based on the given StatType and data
// in the format of /proc/stat.  NewStat returns error if it cannot parse Stat
// properly.
func NewStat(what StatType, data []byte) (*Stat, error) {
	lines := bytes.Split(data, []byte{'\n'})
	s := &Stat{}

	for _, line := range lines {
		// Stop when there is nothing more to read
		if what == 0 {
			break
		}
		x := bytes.IndexByte(line, ' ')
		if x < 0 {
			continue // should we return an error?
		}

		var fields [][]byte

		// Classic atoi for unsigned int64
		atoi := func(w []byte) int64 {
			var v int64
			for _, d := range w {
				if d < '0' || d > '9' {
					return v
				}
				v = v*10 + int64(d-'0')
			}
			return v
		}

		readField := func(i int) int64 {
			if len(fields) <= i {
				return 0
			}
			return atoi(fields[i])
		}

		switch {
		case bytes.Equal(line[:3], []byte("cpu")):
			switch {
			case x == 3 && (what&StatCPU) != 0:
				what &= ^StatCPU
			case what&StatCPUs != 0:
			default:
				continue
			}
			cpu := &CPU{}
			fields = bytes.Fields(line)
			cpu.User = readField(1)
			cpu.Nice = readField(2)
			cpu.System = readField(3)
			cpu.Idle = readField(4)
			cpu.IOWait = readField(5)
			cpu.IRQ = readField(6)
			cpu.SoftIRQ = readField(7)
			cpu.Steal = readField(8)
			cpu.Guest = readField(9)
			cpu.GuestNice = readField(10)
			cpu.Total = cpu.User +
				cpu.Nice +
				cpu.System +
				cpu.Idle +
				cpu.IOWait +
				cpu.IRQ +
				cpu.SoftIRQ +
				cpu.Steal +
				cpu.Guest +
				cpu.GuestNice
			s.CPUs = append(s.CPUs, cpu)
		case bytes.Equal(line[:x], []byte("intr")):
			if what&StatIRQ == 0 {
				break
			}
			fields = bytes.Fields(line)
			s.Interrupts = make([]int64, 0, len(fields)-1)
			for _, w := range fields[1:] {
				s.Interrupts = append(s.Interrupts, atoi(w))
			}
		case bytes.Equal(line[:x], []byte("softirq")):
			if what&StatIRQ == 0 {
				break
			}
			fields = bytes.Fields(line)
			s.SoftInterrupts = make([]int64, 0, len(fields)-1)
			for _, w := range fields[1:] {
				s.SoftInterrupts = append(s.SoftInterrupts, atoi(w))
			}
		case bytes.Equal(line[:x], []byte("btime")):
			if what&StatBootTime == 0 {
				break
			}
			what &= ^StatCPU
			bt := atoi(line[x+1:])
			if bt != 0 {
				s.BootTime = time.Unix(int64(bt), 0)
			}
		case bytes.Equal(line[:x], []byte("ctxt")):
			if what&StatProcess == 0 {
				break
			}
			s.ContextSwitches = atoi(line[x+1:])
		case bytes.Equal(line[:x], []byte("processes")):
			if what&StatProcess == 0 {
				break
			}
			s.Processes = atoi(line[x+1:])
		case bytes.Equal(line[:x], []byte("procs_running")):
			if what&StatProcess == 0 {
				break
			}
			s.Runnable = atoi(line[x+1:])
		case bytes.Equal(line[:x], []byte("procs_blocked")):
			if what&StatProcess == 0 {
				break
			}
			s.Blocked = atoi(line[x+1:])
		}
	}
	return s, nil
}

// Usage returns the CPU usage for user, system, idle, wait and guest times as
// values between 0 and 1.  User time is the sum of the User and Nice fields.
// System time includes the System, IRQ and SoftIRQ fields.  Wait time includes
// the IOWait and Steal fields.  Guest time include the Guest and GuestNice
// fields.
func (cpu *CPU) Usage() (user, system, idle, wait, guest float64) {
	if cpu.Total == 0 {
		return 0, 0, 0, 0, 0
	}
	t := float64(cpu.Total)
	return float64(cpu.User+cpu.Nice) / t,
		float64(cpu.System+cpu.IRQ+cpu.SoftIRQ) / t,
		float64(cpu.Idle) / t,
		float64(cpu.IOWait+cpu.Steal) / t,
		float64(cpu.Guest+cpu.GuestNice) / t
}

// Delta returns a new CPU that is equal to cpu - old.
func (cpu *CPU) Delta(old *CPU) *CPU {
	return &CPU{
		Total:     cpu.Total - old.Total,
		User:      cpu.User - old.User,
		Nice:      cpu.Nice - old.Nice,
		System:    cpu.System - old.System,
		Idle:      cpu.Idle - old.Idle,
		IOWait:    cpu.IOWait - old.IOWait,
		IRQ:       cpu.IRQ - old.IRQ,
		SoftIRQ:   cpu.SoftIRQ - old.SoftIRQ,
		Steal:     cpu.Steal - old.Steal,
		Guest:     cpu.Guest - old.Guest,
		GuestNice: cpu.GuestNice - old.GuestNice,
	}
}

// A ProcessStat structure contains the information read from /proc/PID/stat.
type ProcessStat struct {
	Pid              int           // The process ID
	Command          string        // Filename of the executable
	State            string        // Process state
	PPid             int           // Parent process ID
	PGid             int           // Process group ID
	SessionID        int           // Session ID
	ControllingTTY   int           // Controlling TTY
	TTYProcessGroup  int           // Foreground process group of controlling tty
	Flags            uint          // Linux kernel flags
	MinorFaults      uint64        // Faults not requiring a pagein
	ChildMinorFaults uint64        // Faults from waited children
	MajorFaults      uint64        // Faults requiring a pagein
	ChildMajorFaults uint64        // Faults from waited children
	UserTime         time.Duration // Time spent in user/guest mode
	SystemTime       time.Duration // Time spent in system mode
	ChildUserTime    time.Duration // Waited childrens user/guest time
	ChildSystemTime  time.Duration // Waited childrens system time
	Priority         int64         // Process priority
	Nice             int64         // Process nice level (19 to -20)
	Threads          int           // Number of threads in process
	StartTime        time.Duration // duration since system boot
	VMSize           uint64        // Virtual Memory Size in bytes
	RSSSize          int64         // Resident Set Size (pages of real memory)
	RSSLimit         uint64        // RSS Soft limit
	StartCode        uint64        // Lowest executable text address
	EndCode          uint64        // Highest executable text address
	StartStack       uint64        // Address (bottom) of the stack
	KernelSP         uint64        // Current kernel stack pointer
	KernelIP         uint64        // Current kernel instruction pointer
	PendingSignals   uint64        // Bitmap of pending signals
	BlockedSignals   uint64        // Bitmap of blocked signals
	IgnoredSignals   uint64        // Bitmap of ignored signals
	CaughtSignals    uint64        // Bitmap of caught signals (obsolete)
	WaitChan         uint64        // Address where process is sleeping
	ExitSignal       int           // Signal sent when we die
	LastCPU          int           // Last CPU process executed on
	RTPriority       uint          // Real-time scheduling priority
	SchedPolicy      uint          // Scheduling Policy
	BlockIODelays    time.Duration // Aggregated time blocked on I/O
	GuestTime        time.Duration // Aggregated time in guest mode
	GuestChildTime   time.Duration // Aggregated time of children in gues mode
	StartData        uint64
	EndData          uint64
	StartBreak       uint64
}

// stateMap maps single letter states into human names
var stateMap = map[string]string{
	"R": "Running",
	"S": "Sleeping",
	"D": "Disk Sleep",
	"Z": "Zombie",
	"T": "Traced/Stopped",
	"W": "Paging",
}

// ProcStat returns the contents of /proc/PID/stat as a ProcessStat.
func ProcStat(pid int) (_ *ProcessStat, err error) {
	data, err := ioutil.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("process %d missing stat data", pid)
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// The data from the stat file consists of a single line of space
	// separated fields.  47 fields were defined in the production linux
	// kernel at the time this module was written.
	fields := strings.Fields(string(data))
	n := len(fields)

	// We use a panic with a procError to bail out of processing when we
	// hit a processing error in a subfunction.
	type procError struct{ err error }
	defer func() {
		if p := recover(); p != nil {
			if pe, ok := p.(procError); ok {
				err = pe.err
			}
		}
	}()

	// The get* functions return the next (last) field of input as a
	// string, uint64, int64, uint, int, or time.Duration.  We process the
	// fields in reverse order.
	getS := func() string {
		n--
		return fields[n]
	}

	getLU := func() uint64 {
		if v, err := strconv.ParseUint(getS(), 10, 64); err != nil {
			panic(procError{err})
		} else {
			return v
		}
	}

	getLD := func() int64 {
		if v, err := strconv.ParseInt(getS(), 10, 64); err != nil {
			panic(procError{err})
		} else {
			return v
		}
	}

	getU := func() uint { return uint(getLU()) }
	getD := func() int { return int(getLD()) }

	// Some fields are documented to be in terms of _SC_CLK_TICK while
	// others are documented to be in terms of 1/100ths of a second.
	getDuration := func() time.Duration {
		return TickDuration * time.Duration(getLU())
	}
	getDuration100 := func() time.Duration {
		return (time.Second / 100) * time.Duration(getLU())
	}

	var p ProcessStat
	switch n {
	case 0:
		// it is not possible for strings.Fields to return a value that
		// has a length of 0.
		panic("unreachable")
	default:
		// We only understand the first 47 fields.  We need to trim
		// off the trailing fields (if they exist) so that the getLU
		// below will return the 47th field.
		n = 47
		fallthrough
	case 47:
		p.StartBreak = getLU()
		fallthrough
	case 46:
		p.EndData = getLU()
		fallthrough
	case 45:
		p.StartData = getLU()
		fallthrough
	case 44:
		p.GuestChildTime = getDuration()
		fallthrough
	case 43:
		p.GuestTime = getDuration()
		fallthrough
	case 42:
		p.BlockIODelays = getDuration100()
		fallthrough
	case 41:
		p.SchedPolicy = getU()
		fallthrough
	case 40:
		p.RTPriority = getU()
		fallthrough
	case 39:
		p.LastCPU = getD()
		fallthrough
	case 38:
		p.ExitSignal = getD()
		fallthrough
	case 37:
		getS() // cnswap is not maintained
		fallthrough
	case 36:
		getS() // nswap is not maintained
		fallthrough
	case 35:
		p.WaitChan = getLU()
		fallthrough
	case 34:
		p.CaughtSignals = getLU()
		fallthrough
	case 33:
		p.IgnoredSignals = getLU()
		fallthrough
	case 32:
		p.BlockedSignals = getLU()
		fallthrough
	case 31:
		p.PendingSignals = getLU()
		fallthrough
	case 30:
		p.KernelIP = getLU()
		fallthrough
	case 29:
		p.KernelSP = getLU()
		fallthrough
	case 28:
		p.StartStack = getLU()
		fallthrough
	case 27:
		p.EndCode = getLU()
		fallthrough
	case 26:
		p.StartCode = getLU()
		fallthrough
	case 25:
		p.RSSLimit = getLU()
		fallthrough
	case 24:
		p.RSSSize = getLD()
		fallthrough
	case 23:
		p.VMSize = getLU()
		fallthrough
	case 22:
		p.StartTime = getDuration()
		fallthrough
	case 21:
		getS() // itrealvalue is not maintained
		fallthrough
	case 20:
		p.Threads = getD()
		fallthrough
	case 19:
		p.Nice = getLD()
		fallthrough
	case 18:
		p.Priority = getLD()
		fallthrough
	case 17:
		p.ChildSystemTime = getDuration()
		fallthrough
	case 16:
		p.ChildUserTime = getDuration()
		fallthrough
	case 15:
		p.SystemTime = getDuration()
		fallthrough
	case 14:
		p.UserTime = getDuration()
		fallthrough
	case 13:
		p.ChildMajorFaults = getLU()
		fallthrough
	case 12:
		p.MajorFaults = getLU()
		fallthrough
	case 11:
		p.ChildMinorFaults = getLU()
		fallthrough
	case 10:
		p.MinorFaults = getLU()
		fallthrough
	case 9:
		p.Flags = getU() // see sched.h
		fallthrough
	case 8:
		p.TTYProcessGroup = getD()
		fallthrough
	case 7:
		// The minor device number is contained in the combination of
		// bits 31 to 20 and 7 to 0; the major device number is in
		// bits 15 to 8.)
		p.ControllingTTY = getD()
		fallthrough
	case 6:
		p.SessionID = getD()
		fallthrough
	case 5:
		p.PGid = getD()
		fallthrough
	case 4:
		p.PPid = getD()
		fallthrough
	case 3:
		p.State = getS()
		if s := stateMap[p.State]; s != "" {
			p.State = s
		}
		fallthrough
	case 2:
		p.Command = getS()
		if len(p.Command) > 1 {
			if p.Command[0] == '(' && p.Command[len(p.Command)-1] == ')' {
				p.Command = p.Command[1 : len(p.Command)-1]
			}
		}
		fallthrough
	case 1:
		p.Pid = getD()
	}
	return &p, nil
}

// ProcStartTime returns the start time of a process as a duration since
// system boot.  This value can be used a pseudo-generation number for a
// given process ID.
func ProcStartTime(pid int) (time.Duration, error) {
	data, err := ioutil.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("process %d missing stat data", pid)
	}
	fields := strings.Fields(string(data))

	// We know StartTime to be the 22nd field.
	if len(fields) < 22 {
		return 0, fmt.Errorf("process %d truncated stat data", pid)
	}
	n, err := strconv.ParseUint(fields[21], 10, 64)
	if err != nil {
		return 0, err
	}
	return TickDuration * time.Duration(n), nil
}
