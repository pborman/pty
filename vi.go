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
	"path"
	"strings"

	pspkg "github.com/pborman/ps"
)

func PS(pid int) string {
	pm, err := pspkg.GetProcessMap()
	if err != nil {
		return err.Error()
	}
	p := pm.Pids[pid]
	if p == nil {
		return "process not found"
	}
	var buf bytes.Buffer
	printProc(&buf, p, "")
	return buf.String()
}

func sanePath(p string) string {
	if strings.HasPrefix(p, user.HomeDir) {
		return "~" + p[len(user.HomeDir):]
	}
	return p
}

func processName(p *pspkg.Process) string {
	cmd, err := p.Command()
	if err == nil && cmd != "" {
		return path.Base(cmd)
	}
	argv, err := p.Argv()
	if err == nil && len(argv) > 0 {
		return path.Base(strings.TrimPrefix(argv[0], "-"))
	}
	return ""
}

func processWD(p *pspkg.Process) string {
	wd, err := p.Cwd()
	if err != nil || wd == "" {
		return "unknown"
	}
	return wd
}

func processFiles(p *pspkg.Process) []string {
	fds, err := p.Fds()
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(fds))
	for _, fd := range fds {
		if fd.Path != "" {
			files = append(files, fd.Path)
		}
	}
	return files
}

func printProc(w io.Writer, p *pspkg.Process, prefix string) {
	name := processName(p)
	wd := sanePath(processWD(p))
	argv, _ := p.Argv()
	switch name {
	case "pty":
		fmt.Fprintf(w, "%s pty %d (%s)\n", prefix, p.Pid(), wd)
	case "vi", "vi.exe", "vim", "nvim", "govi", "nvi:
		fmt.Fprintf(w, "%svi %s (%s)\n", prefix, viFiles(argv, processFiles(p)), wd)
	default:
		if len(argv) == 0 {
			fmt.Fprintf(w, "%s%s (%s)\n", prefix, name, wd)
		} else {
			fmt.Fprintf(w, "%s%s (%s)\n", prefix, argv, wd)
		}
	}
	if prefix == "" {
		prefix = "\u2b11 "
	}
	for _, child := range p.Children {
		printProc(w, child, "  "+prefix)
	}
}

func viFiles(argv, files []string) []string {
	var out []string
	a := argv
	if len(a) > 0 {
		a = a[1:]
	}
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		a = a[1:]
	}
	for _, file := range a {
		out = append(out, sanePath(file))
	}
	for _, file := range files {
		switch {
		case !strings.HasPrefix(file, "/"):
		case strings.HasPrefix(file, "/dev"):
		case strings.HasPrefix(file, "/private/dev"):
		case strings.HasPrefix(file, "/tmp/vi."):
		case strings.HasPrefix(file, "/private/tmp/vi."):
		case strings.HasPrefix(file, "/var/tmp/vi.recover"):
		case strings.HasPrefix(file, "/private/var/tmp/vi.recover"):
		case strings.Contains(file, "/.vim/swap"):
		default:
			out = append(out, sanePath(file))
		}
	}
	return out
}
