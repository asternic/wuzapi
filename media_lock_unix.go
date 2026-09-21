//go:build !windows

package main

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockMediaFile(f *os.File, nonblocking bool) error {
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), flags)
}
