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
	"fmt"
	"strings"
	"testing"
)

func (p *Process) Print(prefix string) {
	p.Fill()
	fmt.Printf("%s%-10d %s\n", prefix, p.Pid, p.Name)
	fmt.Printf("%s%10s - pwd: %s\n", prefix, "", p.WD)
	fmt.Printf("%s%10s - argv: %q\n", prefix, "", p.Argv)
	fmt.Printf("%s%10s - sid: %d\n", prefix, "", p.SessionID)
	if ps, _ := ProcStat(p.Pid); ps != nil {
		fmt.Printf("%s%10s - age: %s\n", prefix, "", ps.StartTime)
	}
	for _, file := range p.Files {
		fmt.Printf("%s%10s - file: %s\n", prefix, "", file)
	}
	for _, child := range p.Children {
		child.Print(prefix + "  ")
	}
}

func (p *Process) VIFiles() []string {
	var files []string
	a := p.Argv[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		a = a[1:]
	}
	files = append(files, a...)
	for _, file := range p.Files {
		switch {
		case !strings.HasPrefix(file, "/"):
		case strings.HasPrefix(file, "/dev"):
		case strings.HasPrefix(file, "/var/tmp/vi.recover"):
		default:
			files = append(files, file)
		}
	}
	return files
}

func TestPS(t *testing.T) {
	tree, err := NewProcessTree()
	if err != nil {
		t.Fatal(err)
	}
	tree.Pids[1].Print("")
}
