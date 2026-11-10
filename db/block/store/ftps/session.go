package block_store_ftps

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/errors"
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
	// done is closed once every socket of the session is confirmed closed.
	done chan struct{}
}

// newSession returns a session whose data connections use tlsConf.
func newSession(tlsConf *tls.Config) *session {
	return &session{tlsConf: tlsConf, done: make(chan struct{})}
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
// closeConfirmWait passed. A second close waits for the first.
func (s *session) close() {
	// Mark the session closed and take its sockets, or wait for the close
	// already under way.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		<-s.done
		return
	}
	s.closed = true
	conns := s.conns
	s.conns = nil
	s.mtx.Unlock()
	defer close(s.done)

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

// ErrSessionsClosed is returned by every call of a Client after CloseAll,
// and by a call that was running when CloseAll cut its session.
var ErrSessionsClosed = errors.New("ftps sessions closed by CloseAll")

// sessions tracks every session of a client, those being dialed included,
// and the ServerConn of each logged in one.
type sessions struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// dead is set by closeAll: no session is tracked after it.
	dead bool
	// all holds every session not yet forgotten.
	all map[*session]struct{}
	// byConn maps each logged in ServerConn to its session.
	byConn map[*ftp.ServerConn]*session
}

// track records a session about to dial, or refuses it after closeAll.
func (ss *sessions) track(s *session) error {
	// Refuse the session once closeAll ran.
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	if ss.dead {
		return ErrSessionsClosed
	}

	// Record it, making the maps on first use.
	if ss.all == nil {
		ss.all = make(map[*session]struct{})
		ss.byConn = make(map[*ftp.ServerConn]*session)
	}
	ss.all[s] = struct{}{}
	return nil
}

// add records the ServerConn of a tracked session once it logged in.
func (ss *sessions) add(conn *ftp.ServerConn, s *session) {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	ss.byConn[conn] = s
}

// forget drops a session whose dial failed.
func (ss *sessions) forget(s *session) {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	delete(ss.all, s)
}

// take removes conn and its session, returning the session.
func (ss *sessions) take(conn *ftp.ServerConn) *session {
	// Look up and drop the session under the lock.
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	s := ss.byConn[conn]
	delete(ss.byConn, conn)
	delete(ss.all, s)
	return s
}

// quit logs conn out and forgets its session.
func (ss *sessions) quit(conn *ftp.ServerConn) {
	ss.take(conn)
	_ = conn.Quit()
}

// kill closes the sockets of conn's session, waiting for their close, and
// forgets it.
func (ss *sessions) kill(conn *ftp.ServerConn) {
	if s := ss.take(conn); s != nil {
		s.close()
	}
}

// isDead reports whether closeAll ran.
func (ss *sessions) isDead() bool {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	return ss.dead
}

// closeAll refuses every later session and closes the sockets of every
// session now tracked at once, those being dialed and those running a call
// included, waiting until each is confirmed closed. The sessions stay
// tracked, so a kill of one waits for its close too.
func (ss *sessions) closeAll() {
	// Refuse new sessions and take the tracked ones.
	ss.mtx.Lock()
	ss.dead = true
	all := make([]*session, 0, len(ss.all))
	for s := range ss.all {
		all = append(all, s)
	}
	ss.mtx.Unlock()

	// Close them in parallel.
	var wg sync.WaitGroup
	for _, s := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.close()
		}()
	}
	wg.Wait()
}
