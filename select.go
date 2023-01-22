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
	"errors"
	"fmt"
	"os"
	osuser "os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	prefix      = "session-"
	titlePrefix = "title-"
	pidPrefix   = "pid-"
	debugSuffix = ".debug"
	fwdSuffix   = ".forward"
	rcdir       = ".pty"
)

var (
	user       *osuser.User
	removedErr = errors.New("removed")
)

func init() {
	var err error
	user, err = osuser.Current()
	if err != nil {
		exitf("Getting current user: %v", err)

	}
	if user.HomeDir == "" {
		exitf("%s has no home directory", user.Username)
	}
}

func splur(s int) string {
	if s == 1 {
		return ""
	}
	return "s"
}

var loginShell = os.Getenv("SHELL")

func execsh() {
	sh := "-" + filepath.Base(loginShell)
	err := syscall.Exec(loginShell, []string{sh}, os.Environ())
	exitf("exec failed with %v", err)
}

// Select session returns the path to the selected session.  If the returned
// bool is true then this session must be created.  An error is returned if
// there was an error reading the name of the session.
func SelectSession() (*Session, error) {
	sessions := GetSessions()
	if len(sessions) == 0 {
		if loginShell != "" {
			fmt.Printf("Name of session to create (or shell): ")
		} else {
			fmt.Printf("Name of session to create: ")
		}
		name, err := readline()
		if err != nil {
			exitf("%v", err)
		}
		if loginShell != "" && name == "shell" {
			execsh()
		}
		if !ValidSessionName(name) {
			exitf("invalid session name %q", name)
		}
		s := MakeSession(name)
		if s.Check() {
			exitf("session %q already exists", name)
		}
		return s, nil
	}
	if loginShell != "" {
		fmt.Printf("shell) Spawn %s\n", loginShell)
	}
	fmt.Printf(" name) Create a new session named name\n")
	for i, s := range sessions {
		fmt.Printf("    %d) %s (%d Client%s) %s\n", i+1, s.Name, s.cnt, splur(s.cnt), s.Title())
		if ps := s.PS(); ps != "" {
			for _, line := range strings.Split(ps, "\n") {
				if line == "" {
					continue
				}
				if len(line) > 80 {
					line = line[:80]
				}
				fmt.Printf("        %s\n", line)
			}
		}
	}
Loop:
	for {
		fmt.Printf("Please select a session: ")
		name, err := readline()
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, nil
		}
		if name == "shell" {
			if loginShell == "" {
				exitf("$SHELL not set.")
			}
			execsh()
			exitf("failed to exec %v", loginShell)

		}
		if n, err := strconv.Atoi(name); err == nil {
			if n >= 1 && n <= len(sessions) {
				return sessions[n-1], nil
			}
			fmt.Printf("Select a number between 1 and %d\n", len(sessions))
		} else {
			if !ValidSessionName(name) {
				fmt.Printf("%q is an invalid session name\n", name)
				continue Loop
			}
			for _, s := range sessions {
				if name == s.Name {
					return s, nil
				}
			}
			ok, err := readYesNo("Create session %s [Y/N]? ", name)
			switch {
			case err != nil:
				return nil, err
			case ok:
				return MakeSession(name), nil
			default:
				continue Loop
			}
		}
	}
}

func readYesNo(format string, v ...interface{}) (bool, error) {
	for {
		fmt.Printf(format, v...)
		answer, err := readline()
		switch {
		case err != nil:
			return false, err
		case answer == "y" || answer == "Y":
			return true, nil
		case answer == "n" || answer == "N":
			return false, nil
		}
	}
}

func readline() (string, error) {
	// lines must be shorter than 256 bytes
	var buf [256]byte
	for i := 0; ; i++ {
		if i == len(buf) {
			i--
		}
		_, err := os.Stdin.Read(buf[i : i+1])
		if err != nil {
			return "", err
		}
		if buf[i] == '\n' || buf[i] == '\r' {
			return string(bytes.TrimSpace(buf[:i])), nil
		}
	}
}

func GetSessions() []*Session {
	dir := filepath.Join(user.HomeDir, rcdir)
	fd, err := os.Open(dir)
	if err != nil {
		warnf("finding session names: %v", err)
		return nil
	}
	dirs, _ := fd.Readdirnames(-1)
	checkClose(fd)
	ch := make(chan *Session, len(dirs))
	var wg sync.WaitGroup

	for _, name := range dirs {
		if name == "log" {
			continue
		}
		s := MakeSession(name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !s.Check() {
				return
			}
			ch <- s
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var sessions []*Session
	for s := range ch {
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})
	return sessions
}
