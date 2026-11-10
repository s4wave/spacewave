package main

import "golang.org/x/sys/unix"

// getLockCmd tests for a lock as an open file description lock, which
// conflicts with every POSIX lock, this process's own included.
const getLockCmd = unix.F_OFD_GETLK
