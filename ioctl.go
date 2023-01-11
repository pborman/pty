package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var ENOTTY = errors.New("not a tty")

func ioctl(fd, cmd, data uintptr) error {
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, data); err != 0 {
		return err
	}
	return nil
}

func setsize(f *os.File, rows, cols int) error {
	ws := &struct {
		rows   uint16
		cols   uint16
		xpixel uint16
		ypixel uint16
	}{
		rows: uint16(rows),
		cols: uint16(cols),
	}
	err := ioctl(f.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(ws)))

	if e, ok := err.(syscall.Errno); ok && e == syscall.ENOTTY {
		// Don't return "inappropriate ioctl for device"
		return ENOTTY
	}
	return err
}
