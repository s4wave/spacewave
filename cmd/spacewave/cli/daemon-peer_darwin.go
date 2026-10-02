//go:build darwin

package spacewave_cli

import (
	"bytes"
	"net"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// peerProcessName returns the executable name of the process on the other end
// of a Unix socket, or an empty string when the peer cannot be identified.
func peerProcessName(conn net.Conn) string {
	// Read the peer process ID from the socket credentials.
	pid, ok := unixPeerPID(conn, func(fd int) (int, error) {
		return unix.GetsockoptInt(fd, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	})
	if !ok {
		return ""
	}

	// kern.procargs2 holds argc and then the full executable path.
	if args, err := unix.SysctlRaw("kern.procargs2", pid); err == nil && len(args) > 4 {
		path, _, _ := bytes.Cut(args[4:], []byte{0})
		if len(path) != 0 {
			return filepath.Base(string(path))
		}
	}

	// The kernel's command name is truncated but needs no extra access.
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	return unix.ByteSliceToString(proc.Proc.P_comm[:])
}
