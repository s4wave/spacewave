//go:build !js && !darwin && !linux

package spacewave_cli

import "net"

// peerProcessName returns an empty string: this platform does not report the
// process on the other end of a Unix socket.
func peerProcessName(net.Conn) string {
	return ""
}
