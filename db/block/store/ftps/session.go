package block_store_ftps

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
)

// closeConfirmWait bounds the wait for the server to answer a closed
// connection with its own close. A connection it does not answer in time
// still counts as open for that long.
const closeConfirmWait = 10 * time.Second

// session tracks the sockets of one FTP session, its control connection and
// its data connections, so a call that ends at its context can close them
// rather than leave them open behind an abandoned command.
type session struct {
	// tlsConf wraps the data connections in TLS.
	tlsConf *tls.Config
	// mtx guards the fields below.
	mtx sync.Mutex
	// conns are the session's sockets, the control connection first.
	conns []net.Conn
	// dialed is set once the control connection is dialed.
	dialed bool
	// closed is set once the session is closed: a later dial is refused.
	closed bool
}

// dialFunc returns the dial function of the session: the first dial is the
// control connection, which the client upgrades with AUTH TLS itself, and
// each later one a data connection, wrapped here in TLS.
func (s *session) dialFunc(ctx context.Context) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		// Tell the control connection, the first dial, from a data one.
		s.mtx.Lock()
		first := !s.dialed
		s.dialed = true
		s.mtx.Unlock()

		// Dial the socket: the control connection under the dial's context,
		// a data connection, dialed by a later call, under dialTimeout alone.
		dctx := ctx
		if !first {
			dctx = context.WithoutCancel(ctx)
		}
		d := &net.Dialer{Timeout: dialTimeout}
		conn, err := d.DialContext(dctx, network, addr)
		if err != nil {
			return nil, err
		}

		// Track it, refusing it when the session is already closed.
		s.mtx.Lock()
		if s.closed {
			s.mtx.Unlock()
			_ = conn.Close()
			return nil, net.ErrClosed
		}
		s.conns = append(s.conns, conn)
		s.mtx.Unlock()
		if first {
			return conn, nil
		}
		return tls.Client(conn, s.tlsConf), nil
	}
}

// close closes every socket of the session and waits until each is
// confirmed closed: the server answered the close with its own, or
// closeConfirmWait passed.
func (s *session) close() {
	// Mark the session closed and take its sockets.
	s.mtx.Lock()
	s.closed = true
	conns := s.conns
	s.conns = nil
	s.mtx.Unlock()

	// Close each and wait for the server's answer in parallel.
	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeConfirmed(conn)
		}()
	}
	wg.Wait()
}

// closeConfirmed half-closes conn, reads until the server closes its side or
// closeConfirmWait passes, then closes it.
func closeConfirmed(conn net.Conn) {
	// Send our close and stop anything blocked writing.
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return
	}
	_ = tcp.CloseWrite()

	// Read until the server's close, discarding what it still sends.
	deadline := time.Now().Add(closeConfirmWait)
	_ = tcp.SetReadDeadline(deadline)
	buf := make([]byte, 4096)
	for {
		if _, err := tcp.Read(buf); err != nil {
			break
		}
	}

	// Close the socket. One the server never answered was counted open for
	// the whole wait.
	_ = tcp.Close()
}

// sessions maps each live ServerConn to its session.
type sessions struct {
	m sync.Map
}

// add records the session of conn.
func (ss *sessions) add(conn *ftp.ServerConn, s *session) {
	ss.m.Store(conn, s)
}

// quit logs conn out and forgets its session.
func (ss *sessions) quit(conn *ftp.ServerConn) {
	ss.m.Delete(conn)
	_ = conn.Quit()
}

// kill closes the sockets of conn's session, waiting for their close, and
// forgets it.
func (ss *sessions) kill(conn *ftp.ServerConn) {
	if v, ok := ss.m.LoadAndDelete(conn); ok {
		v.(*session).close()
	}
}
