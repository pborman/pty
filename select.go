package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	osuser "os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	prefix      = "session-"
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

// Select session returns the path to the selected session.  If the returned
// bool is true then this session must be created.  An error is returned if
// there was an error reading the name of the session.
func SelectSession() (name string, _ bool, err error) {
	defer func() {
		// return the full path name of the session.
		if name != "" {
			name = filepath.Join(user.HomeDir, rcdir, prefix+name)
		}
	}()
	sessions := GetSessions()
	if len(sessions) == 0 {
		fmt.Printf("Name of session to create: ")
		name, err = readline()
		return name, name != "", err
	}
	fmt.Printf("Current sessions:\n")
	for i, si := range sessions {
		fmt.Printf("    %d) %s (%d)\n", i+1, si.Name, si.Count)
		if si.PS != "" {
			for _, line := range strings.Split(si.PS, "\n") {
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
			return "", false, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return "", false, nil
		}
		if n, err := strconv.Atoi(name); err == nil {
			if n >= 1 && n <= len(sessions) {
				return sessions[n-1].Name, false, nil
			}
			fmt.Printf("Select a number between 1 and %d\n", len(sessions))
		} else {
			for _, si := range sessions {
				if name == si.Name {
					return name, false, nil
				}
			}
			ok, err := readYesNo("Create session %s [Y/N]? ", name)
			switch {
			case err != nil:
				return "", false, err
			case ok:
				return name, true, nil
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

type SessionInfo struct {
	Count int
	Name  string
	PS    string
}

func GetSessions() []SessionInfo {
	dir := filepath.Join(user.HomeDir, rcdir)
	fd, err := os.Open(dir)
	if err != nil {
		warnf("finding session names: %v", err)
		return nil
	}
	dirs, _ := fd.Readdirnames(-1)
	fd.Close()
	ch := make(chan SessionInfo)

	var wg sync.WaitGroup

	for _, name := range dirs {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if strings.HasSuffix(name, debugSuffix) {
			continue
		}
		if strings.HasSuffix(name, fwdSuffix) {
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSocket == 0 {
			continue
		}
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			cnt, pid, err := CheckSession(path)
			switch {
			case err == removedErr:
			case err != nil:
				fmt.Println(err)
			case pid == 0:
				ch <- SessionInfo{cnt, name[len(prefix):], ""}
			default:
				ch <- SessionInfo{cnt, name[len(prefix):], PS(pid)}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var infos []SessionInfo
	for si := range ch {
		infos = append(infos, si)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

func SessionName(socket string) string {
	return strings.TrimPrefix(filepath.Base(socket), prefix)
}

func SessionPath(session string) string {
	return filepath.Join(user.HomeDir, rcdir, prefix+session)
}

func CheckSession(socket string) (cnt, pid int, err error) {
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	if err != nil {
		os.Remove(socket)
		if strings.Contains(err.Error(), "connect: connection refused") {
			return 0, 0, removedErr
		}
		return 0, 0, err
	}
	defer client.Close()
	w := NewMessengerWriter(client)
	w.Sendf(askCountMessage, "")
	ch := make(chan string, 2)
	r := NewMessengerReader(client, func(kind messageKind, data []byte) {
		switch kind {
		case countMessage:
			ch <- string(data)
		}
	})
	go func() {
		var buf [256]byte
		for {
			_, err := r.Read(buf[:])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-time.After(time.Second * 5):
		return 0, 0, fmt.Errorf("%s timed out", socket)
	case msg := <-ch:
		if len(msg) == 1 {
			return int(msg[0]), 0, nil
		}
		x := strings.Index(msg, ":")
		if x < 0 {
			return 0, 0, nil
		}
		cnt, err := strconv.Atoi(msg[:x])
		pid, perr := strconv.Atoi(msg[x+1:])
		if err == nil {
			err = perr
		}
		return cnt, pid, err
	}
}
