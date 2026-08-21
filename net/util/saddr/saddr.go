package saddr

import "net"

// StringAddr is a net.Addr backed by a string.
type StringAddr struct {
	net  string
	addr string
}

// NewStringAddr constructs a new net.Addr from strings.
func NewStringAddr(net, addr string) *StringAddr {
	return &StringAddr{
		net:  net,
		addr: addr,
	}
}

// Network returns the network name.
func (s *StringAddr) Network() string {
	return s.net
}

// String returns the address string.
func (s *StringAddr) String() string {
	return s.addr
}

// _ is a type assertion
var _ net.Addr = (*StringAddr)(nil)
