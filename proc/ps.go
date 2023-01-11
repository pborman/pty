package proc

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Process struct {
	Pid       int
	PPid      int
	Name      string
	Children  []*Process
	Argv      []string
	WD        string
	Files     []string
	SessionID int
	filled    bool
}

func DirectoryList(dir string) ([]string, error) {
	fd, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	return fd.Readdirnames(-1)
}

type ProcessTree struct {
	Pids     map[int]*Process
	Children map[int][]int
}

func Me() (*Process, error) {
	return PS(os.Getpid())
}

func PS(pid int) (*Process, error) {
	t, err := NewProcessTree()
	if err != nil {
		return nil, err
	}
	p := t.Pids[pid]
	if p == nil {
		return nil, fmt.Errorf("process is lost")
	}
	p.Fill()
	return p, nil
}

func NewProcessTree() (*ProcessTree, error) {
	p := &ProcessTree{
		Pids:     map[int]*Process{},
		Children: map[int][]int{},
	}

	procfiles, err := DirectoryList("/proc")
	if err != nil {
		return nil, err
	}
	for _, spid := range procfiles {
		pid, err := strconv.Atoi(spid)
		if err != nil {
			continue
		}
		st, err := GetStatus(pid)
		if err != nil {
			continue
		}
		if _, ok := p.Pids[pid]; ok {
			fmt.Printf("Error: already saw %d\n", pid)
			continue
		}
		p.Pids[pid] = &Process{
			Pid:  pid,
			PPid: int(st.PPid),
			Name: st.Name,
		}
		p.Children[int(st.PPid)] = append(p.Children[int(st.PPid)], pid)
	}
	for _, ps := range p.Pids {
		for _, child := range p.Children[int(ps.Pid)] {
			if cp := p.Pids[child]; cp != nil {
				ps.Children = append(ps.Children, cp)
			}
		}
	}
	return p, nil
}

func (pt *ProcessTree) Process(pid int) *Process {
	if p := pt.Pids[pid]; p != nil {
		if !p.filled {
			p.Fill()
		}
		return p
	}
	return nil
}

func (p *Process) Fill() {
	if p.filled {
		return
	}
	p.SessionID = sessionID(p.Pid)
	p.WD = cwd(p.Pid)
	p.Argv = argv(p.Pid)
	p.Files = openfiles(p.Pid)
	p.filled = true
	for _, child := range p.Children {
		child.Fill()
	}
}

func sessionID(pid int) int {
	data, err := ioutil.ReadFile("/proc/" + strconv.Itoa(pid) + "/sessionid")
	if err != nil {
		return -1
	}
	sid, err := strconv.Atoi(string(data))
	if err != nil {
		return -1
	}
	return sid
}

func argv(pid int) []string {
	data, err := ioutil.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil
	}
	for n := len(data); n > 0 && data[n-1] == 0; {
		n--
		data = data[:n]
	}
	return strings.Split(string(data), "\000")
}

func cwd(pid int) string {
	wd, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err != nil {
		return "unknown"
	}
	return wd
}

func openfiles(pid int) []string {
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"
	fds, _ := DirectoryList(dir)
	filemap := make(map[string]struct{}, len(fds))
	for _, fd := range fds {
		file, err := os.Readlink(dir + "/" + fd)
		if err == nil && strings.HasPrefix(file, "/") {
			filemap[file] = struct{}{}
		}
	}
	files := make([]string, 0, len(filemap))
	for file := range filemap {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
