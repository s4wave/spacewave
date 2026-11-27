package block_store_ftps

import (
	"bytes"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// linkListener accepts connections that delay every byte the server sends
// by its linkRTT, counting them as control or data connections.
type linkListener struct {
	net.Listener
	// srv holds the delay and the counts.
	srv *testServer
	// data marks the listener of a passive data connection.
	data bool
}

// Accept accepts a connection, counts it open and wraps it in a linkConn.
func (l *linkListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Count it open, and closed once the server closes it.
	s := l.srv
	s.mtx.Lock()
	open, peak := &s.links.ctrlOpen, &s.links.ctrlPeak
	if l.data {
		s.links.data++
		open, peak = &s.links.dataOpen, &s.links.dataPeak
	} else {
		s.links.ctrl++
	}
	*open++
	*peak = max(*peak, *open)
	s.mtx.Unlock()
	closed := func() {
		s.mtx.Lock()
		*open--
		s.mtx.Unlock()
	}
	return newLinkConn(conn, s.linkRTT, closed), nil
}

// linkChunk is a write, stamped with the time it was made.
type linkChunk struct {
	at time.Time
	b  []byte
}

// linkChunks bounds the writes in flight: enough that the delay limits the
// latency and not the bandwidth.
const linkChunks = 4096

// linkConn delays every byte written by delay: a write returns at once and
// reaches the socket delay later, in order, and a close reaches it delay
// after it was made, behind the writes made before it. Reads are not
// delayed: the bytes the client sent before the server accepted wait in the
// socket as they would over a real link.
type linkConn struct {
	net.Conn
	delay time.Duration
	// out holds the writes not yet made on the socket.
	out chan linkChunk
	// closing is closed by Close, at closeAt.
	closing   chan struct{}
	closeAt   time.Time
	closeOnce sync.Once
	// onClose runs once when Close is first called.
	onClose func()
}

// newLinkConn wraps conn, calling onClose at its close.
func newLinkConn(conn net.Conn, delay time.Duration, onClose func()) *linkConn {
	l := &linkConn{
		Conn:    conn,
		delay:   delay,
		out:     make(chan linkChunk, linkChunks),
		closing: make(chan struct{}),
		onClose: onClose,
	}
	go l.writeLoop()
	return l
}

// Write queues a copy of p to reach the socket delay from now.
func (l *linkConn) Write(p []byte) (int, error) {
	select {
	case <-l.closing:
		return 0, net.ErrClosed
	default:
	}
	select {
	case l.out <- linkChunk{at: time.Now(), b: bytes.Clone(p)}:
		return len(p), nil
	case <-l.closing:
		return 0, net.ErrClosed
	}
}

// SetDeadline sets the read deadline only: the writes are made later by
// writeLoop, past a write deadline such as the one TLS sets once it sent its
// close alert.
func (l *linkConn) SetDeadline(t time.Time) error {
	return l.SetReadDeadline(t)
}

// SetWriteDeadline is ignored, as SetDeadline says.
func (l *linkConn) SetWriteDeadline(time.Time) error {
	return nil
}

// writeLoop makes each queued write once its delay passed, and at the close
// makes the rest, then closes the socket delay after the close was made.
func (l *linkConn) writeLoop() {
	send := func(c linkChunk) {
		time.Sleep(time.Until(c.at.Add(l.delay)))
		_, _ = l.Conn.Write(c.b)
	}
	for {
		select {
		case c := <-l.out:
			send(c)
			continue
		case <-l.closing:
		}
		for len(l.out) > 0 {
			send(<-l.out)
		}
		time.Sleep(time.Until(l.closeAt.Add(l.delay)))
		_ = l.Conn.Close()
		return
	}
}

// Close closes the connection: the socket closes delay later, behind the
// writes already made.
func (l *linkConn) Close() error {
	l.closeOnce.Do(func() {
		l.closeAt = time.Now()
		close(l.closing)
		if l.onClose != nil {
			l.onClose()
		}
	})
	return nil
}

// TestDataHandshakeRoundTrip times whole reads of 2 MB and 16 MB objects
// over a link of 85 ms round trip, on one session, with the data
// connections' TLS handshake started as they connect and left to their first
// read as the FTP library does. Starting it early saves one round trip per
// read. Each read takes one data connection and resumes the TLS session; the
// session never holds more than its control and one data connection.
func TestDataHandshakeRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("times reads over an 85 ms link")
	}
	const rtt = 85 * time.Millisecond
	for _, size := range []int{2 << 20, 16 << 20} {
		t.Run(strconv.Itoa(size>>20)+"MB", func(t *testing.T) {
			late := timeReads(t, rtt, size, true)
			early := timeReads(t, rtt, size, false)
			t.Logf("%d MB at %v round trip: %.3f s per read late, %.3f s early, %.2f round trips saved",
				size>>20, rtt, late.Seconds(), early.Seconds(), float64(late-early)/float64(rtt))
			if late-early < rtt/2 {
				t.Fatalf("early handshake saved %v per read; want about %v", late-early, rtt)
			}
		})
	}
}

// timeReads writes one object of size bytes through a server over a link of
// round trip rtt, reads it once to warm the session, and returns the mean
// time of the reads after, checking the sessions, logins, data connections
// and TLS resumptions they made.
func timeReads(t *testing.T, rtt time.Duration, size int, late bool) time.Duration {
	t.Helper()
	const reads = 5
	ctx := t.Context()
	addr, tlsConf, srv := startLinkServer(t, rtt)
	client := NewClient(Config{
		Addr:          addr,
		Logins:        []Login{{User: "user", Password: "pass"}},
		TLSConfig:     tlsConf,
		MaxSessions:   1,
		ConnsPerLogin: 1,
		NoFullRetry:   true,
		CloseOnDone:   true,
		lateDataTLS:   late,
	})
	t.Cleanup(client.Close)

	// Write the object and read it once, which logs in.
	data := bytes.Repeat([]byte("0123456789abcdef"), size/16)
	if err := client.PutObject(ctx, "b", "p", data, ""); err != nil {
		t.Fatal(err)
	}
	read := func() {
		got, err := client.GetObjectMax(ctx, "b", "p", int64(size))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("read %d bytes, not the %d written", len(got), size)
		}
	}
	read()

	// Time the reads on the warm session.
	before := srv.linkCounts()
	start := time.Now()
	for range reads {
		read()
	}
	per := time.Since(start) / reads
	after := srv.linkCounts()

	// One session and one login served every call, each read took one data
	// connection and resumed the TLS session, and no more than the control
	// and one data connection were ever open at once.
	if after.ctrl != 1 || after.ctrlPeak != 1 {
		t.Fatalf("late=%v: %d control connections, %d at once; want 1", late, after.ctrl, after.ctrlPeak)
	}
	if n := srvLogins(srv, "user"); n != 1 {
		t.Fatalf("late=%v: %d logins; want 1", late, n)
	}
	if n := after.data - before.data; n != reads {
		t.Fatalf("late=%v: %d data connections for %d reads", late, n, reads)
	}
	if after.dataPeak != 1 {
		t.Fatalf("late=%v: %d data connections at once; want 1", late, after.dataPeak)
	}
	if n, r := after.handshakes-before.handshakes, after.resumed-before.resumed; n != reads || r != reads {
		t.Fatalf("late=%v: %d TLS handshakes, %d resumed, for %d reads", late, n, r, reads)
	}
	return per
}
