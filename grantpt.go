package main

/*
#define _XOPEN_SOURCE 500
#include <stdlib.h>
#include <errno.h>

// Wrapper function to check errno from Go
int call_grantpt(int fd) {
    if (grantpt(fd) != 0) {
        return errno;
    }
    return 0;
}
*/
import "C"
import (
	"fmt"
	"syscall"
)

// GrantPT calls the C grantpt function on the provided master file descriptor.
func GrantPT(fd uintptr) error {
	// Call the C wrapper function
	errno := C.call_grantpt(C.int(fd))

	if errno != 0 {
		return fmt.Errorf("grantpt failed: %w", syscall.Errno(errno))
	}
	return nil
}
