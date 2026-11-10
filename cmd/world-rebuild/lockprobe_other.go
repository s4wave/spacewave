//go:build !linux

package main

import "golang.org/x/sys/unix"

// getLockCmd tests for a POSIX lock; it does not see this process's own.
const getLockCmd = unix.F_GETLK
