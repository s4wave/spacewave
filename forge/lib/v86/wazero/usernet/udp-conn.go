package usernet

import (
	"net"
	"net/netip"
	"sync"
)

// udpConn relays one guest UDP flow to a host UDP socket.
type udpConn struct {
	stack *Stack
	key   string
	host  net.Conn

	hsrc  [6]byte
	hdest [6]byte
	psrc  netip.Addr
	pdest netip.Addr
	sport uint16
	dport uint16

	mtx    sync.Mutex
	closed bool
}

// newUDPConn builds the relay from the packet's addresses reversed.
func newUDPConn(stack *Stack, key string, host net.Conn, packet *ethPacket) *udpConn {
	return &udpConn{
		stack: stack,
		key:   key,
		host:  host,
		hsrc:  routerMAC,
		hdest: packet.src,
		psrc:  packet.ipv4.dest,
		pdest: packet.ipv4.src,
		sport: packet.ipv4.udp.dport,
		dport: packet.ipv4.udp.sport,
	}
}

// start launches the host-to-guest read loop on its own goroutine; the
func (c *udpConn) start() {
	go c.readLoop()
}

// write forwards guest payload bytes to the host socket.
func (c *udpConn) write(dat []byte) {
	c.mtx.Lock()
	closed := c.closed
	c.mtx.Unlock()
	if closed {
		return
	}
	_, err := c.host.Write(dat)
	if err != nil {
		c.stack.releaseUDP(c.key)
		err = c.close()
		if err != nil {
			c.stack.le.WithError(err).Debug("close udp after write error")
		}
	}
}

// close shuts the host socket exactly once and marks the flow closed.
func (c *udpConn) close() error {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil
	}
	c.closed = true
	c.mtx.Unlock()
	return c.host.Close()
}

// readLoop pumps host datagrams back toward the guest until the host
func (c *udpConn) readLoop() {
	buf := make([]byte, defaultMTU-ipv4HeaderSize-udpHeaderSize)
	for {
		n, err := c.host.Read(buf)
		if n > 0 {
			c.stack.sendUDP(c, buf[:n])
		}
		if err != nil {
			c.stack.releaseUDP(c.key)
			closeErr := c.close()
			if closeErr != nil {
				c.stack.le.WithError(closeErr).Debug("close udp after read error")
			}
			return
		}
	}
}
