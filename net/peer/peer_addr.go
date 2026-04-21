package peer

import (
	"net"
)

// NetAddr matches net.Addr with a peer ID
type NetAddr struct {
	pid ID
}

// NewNetAddr constructs a new net.Addr from a peer ID.
func NewNetAddr(pid ID) net.Addr {
	return &NetAddr{pid: pid}
}

// Network is the name of the network (for example, "tcp", "udp")
func (a *NetAddr) Network() string {
	return "bifrost"
}

// String form of address (for example, "192.0.2.1:25", "[2001:db8::1]:80")
func (a *NetAddr) String() string {
	return a.pid.String()
}

// _ is a type assertion
var _ net.Addr = ((*NetAddr)(nil))
