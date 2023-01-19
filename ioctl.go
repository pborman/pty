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
