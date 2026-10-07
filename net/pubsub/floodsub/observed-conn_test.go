package floodsub

import "net"

// observedConn reports write attempts before the pipe applies backpressure.
type observedConn struct {
	// Conn carries the peer's packet stream.
	net.Conn
	// writes receives write attempts without blocking the transport.
	writes chan struct{}
}

// Write reports the attempt and writes through the pipe.
func (c *observedConn) Write(data []byte) (int, error) {
	// Notify the test that the peer writer reached the transport.
	select {
	case c.writes <- struct{}{}:
	default:
	}

	// Let the pipe block until the remote peer reads or closes.
	return c.Conn.Write(data)
}

// _ is a type assertion.
var _ net.Conn = (*observedConn)(nil)
