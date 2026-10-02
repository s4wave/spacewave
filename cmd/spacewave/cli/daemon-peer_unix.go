//go:build darwin || linux

package spacewave_cli

import (
	"net"
)

// unixPeerPID reads the process ID of the peer of a Unix socket with getPID.
func unixPeerPID(conn net.Conn, getPID func(fd int) (int, error)) (int, bool) {
	// Reach the raw descriptor of a Unix socket.
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, false
	}

	// Ask the kernel for the peer while the descriptor stays open.
	var pid int
	var pidErr error
	if err := raw.Control(func(fd uintptr) {
		pid, pidErr = getPID(int(fd))
	}); err != nil || pidErr != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
