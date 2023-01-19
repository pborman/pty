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

package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/pborman/pty/proc"
)

var (
	firstPS sync.Once
	ptree   *proc.ProcessTree
	perr    error
)

func PS(pid int) string {
	firstPS.Do(func() { ptree, perr = proc.NewProcessTree() })
	if perr != nil {
		return perr.Error()
	}
	p := ptree.Process(pid)
	if p == nil {
		return "process not found"
	}
	var buf bytes.Buffer
	printProc(&buf, p, "")
	return buf.String()
}

func sanePath(path string) string {
	if strings.HasPrefix(path, user.HomeDir) {
		return "~" + path[len(user.HomeDir):]
	}
	return path
}

func printProc(w io.Writer, p *proc.Process, prefix string) {
	switch p.Name {
	case "pty":
		fmt.Fprintf(w, "%s pty %d (%s)\n", prefix, p.Pid, sanePath(p.WD))
	case "vi", "vi.exe":
		fmt.Fprintf(w, "%svi %s (%s)\n", prefix, viFiles(p), sanePath(p.WD))
	default:
		fmt.Fprintf(w, "%s%s (%s)\n", prefix, p.Argv, sanePath(p.WD))
	}
	if prefix == "" {
		prefix = "\u2b11 "
	}
	for _, child := range p.Children {
		printProc(w, child, "  "+prefix)
	}
}

func viFiles(p *proc.Process) []string {
	var files []string
	a := p.Argv[1:]
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		a = a[1:]
	}
	for _, file := range a {
		files = append(files, sanePath(file))
	}
	for _, file := range p.Files {
		switch {
		case !strings.HasPrefix(file, "/"):
		case strings.HasPrefix(file, "/dev"):
		case strings.HasPrefix(file, "/tmp/vi."):
		case strings.HasPrefix(file, "/var/tmp/vi.recover"):
		default:
			files = append(files, sanePath(file))
		}
	}
	return files
}
