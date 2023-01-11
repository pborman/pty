// Package ttyname finds the tty name associate with a *os.File or numeric file
// descriptor on Unix(TM) like systems.  Examples:
//
//	name, err := ttyname.File(os.Stdin)
//	name, err := ttyname.Fileno(0)
package ttyname

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// ttydirs is the list of possible directories the tty device is found in.
// It may be updating for other operating systems.
var ttydirs = []string{
	"/dev/pty", // Linux
	"/dev/pts", // Linux
	"/dev",     // Unix, Darwin, ...
}

var (
	ErrNotTTY   = errors.New("not a tty")
	ErrNotFound = errors.New("tty device not found")
)

func init() {
	// This is a hack to make os.IsNotExist(ErrNotFound) return true.
	// It isn't actually necessary.
	_, err := os.Stat("/\001not\001exist\001")
	if pe, ok := err.(*os.PathError); ok {
		if _, ok := pe.Err.(syscall.Errno); ok {
			ErrNotFound = &os.SyscallError{
				Syscall: "tty",
				Err:     pe.Err,
			}
		}
	}
}

// Fileno either returns the device name of the tty attached to the provided
// file descriptor or an error.  If fileno is not attached to a tty, ErrNotTTY
// is returned.
func Fileno(fileno int) (string, error) {
	return File(os.NewFile(uintptr(fileno), "tty"))
}

// File either returns the device name of the tty attached to the provided
// file descriptor or an error.  If tty is not attached to a tty, ErrNotTTY
// is returned.
func File(tty *os.File) (string, error) {
	fi, err := tty.Stat()
	if err != nil {
		return "", err
	}
	if fi.Mode()&(os.ModeDevice|os.ModeCharDevice) != (os.ModeDevice | os.ModeCharDevice) {
		return "", ErrNotTTY
	}
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unsupported sys stat type %T", sys)
	}
	for _, dir := range ttydirs {
		name, _ := searchDir(dir, sys)
		if name != "" {
			return filepath.Join(dir, name), nil
		}
	}
	return "", ErrNotFound
}

// searchDir searches the directory dir for a file that has the same raw device
// number as in stat.  It either returns the name of the file found or an error.
// searchDir does not descend into other directories.
func searchDir(dir string, stat *syscall.Stat_t) (string, error) {
	fd, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer fd.Close()
	for {
		fis, err := fd.Readdir(256)
		for _, fi := range fis {
			s, ok := fi.Sys().(*syscall.Stat_t)
			if ok && stat.Rdev == s.Rdev {
				return fi.Name(), nil
			}
		}
		switch err {
		case nil:
		case io.EOF:
			return "", nil
		default:
			return "", err
		}
	}
}
