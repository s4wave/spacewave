package browser

import (
	"net"
)

// NetAddr is a message-channel backed net address
type NetAddr struct {
	name string
}

// NewNetAddr constructs a new net.Addr from a peer ID.
func NewNetAddr(name string) net.Addr {
	return &NetAddr{name: name}
}

// Network is the name of the network (for example, "tcp", "udp")
func (a *NetAddr) Network() string {
	return "browser-channel"
}

// String form of address (for example, "192.0.2.1:25", "[2001:db8::1]:80")
func (a *NetAddr) String() string {
	return a.name
}

// _ is a type assertion
var _ net.Addr = ((*NetAddr)(nil))
