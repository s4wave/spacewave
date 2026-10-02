//go:build linux

package spacewave_cli

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// peerProcessName returns the executable name of the process on the other end
// of a Unix socket, or an empty string when the peer cannot be identified.
func peerProcessName(conn net.Conn) string {
	// Read the peer process ID from the socket credentials.
	pid, ok := unixPeerPID(conn, func(fd int) (int, error) {
		cred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			return 0, err
		}
		return int(cred.Pid), nil
	})
	if !ok {
		return ""
	}

	// Prefer the full executable name over the truncated command name.
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	if exe, err := os.Readlink(filepath.Join(proc, "exe")); err == nil {
		return filepath.Base(exe)
	}
	comm, err := os.ReadFile(filepath.Join(proc, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(comm))
}
