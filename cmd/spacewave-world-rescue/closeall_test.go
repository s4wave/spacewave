//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
)

// scriptServer is an FTPS server that logs in two sessions and starts a
// stalled transfer for each RETR: it sends a few bytes and then nothing. It
// refuses the third session with 421 once two transfers are under way, and
// every later one at once. It counts every socket, control and data.
type scriptServer struct {
	t        *testing.T
	listener net.Listener
	tlsConf  *tls.Config
	// streaming is closed once two transfers are under way.
	streaming chan struct{}
	mtx       sync.Mutex
	open      int
	accepted  int
	closed    int
	controls  int
	retrs     int
	started   int
	refusedAt time.Time
}

// newScriptServer starts a scriptServer until the test ends and returns it
// with a client TLS configuration that trusts it.
func newScriptServer(t *testing.T) (*scriptServer, *tls.Config) {
	// Issue a self-signed certificate for 127.0.0.1.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)

	// Listen for control connections.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	s := &scriptServer{
		t: t, listener: l, streaming: make(chan struct{}),
		tlsConf: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			s.opened()
			go s.control(conn)
		}
	}()
	return s, &tls.Config{RootCAs: roots}
}

// opened counts a socket accepted.
func (s *scriptServer) opened() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.open++
	s.accepted++
}

// drain reads conn until the client closes it, then counts it closed and
// closes it.
func (s *scriptServer) drain(conn net.Conn) {
	_, _ = io.Copy(io.Discard, conn)
	s.mtx.Lock()
	s.open--
	s.closed++
	s.mtx.Unlock()
	_ = conn.Close()
}

// counts returns the socket and request counters.
func (s *scriptServer) counts() (open, accepted, closed, controls, retrs int, refusedAt time.Time) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.open, s.accepted, s.closed, s.controls, s.retrs, s.refusedAt
}

// control serves one control connection.
func (s *scriptServer) control(raw net.Conn) {
	// Refuse the third session once two transfers stream, and any later one
	// at once.
	s.mtx.Lock()
	s.controls++
	n := s.controls
	s.mtx.Unlock()
	if n >= 3 {
		if n == 3 {
			select {
			case <-s.streaming:
			case <-time.After(5 * time.Second):
				s.t.Error("two transfers never streamed before the third session")
			}
		}
		s.mtx.Lock()
		if s.refusedAt.IsZero() {
			s.refusedAt = time.Now()
		}
		s.mtx.Unlock()
		_, _ = io.WriteString(raw, "421 Too many connections (2) from this IP\r\n")
		s.drain(raw)
		return
	}

	// Greet, then upgrade to TLS at AUTH TLS.
	_, _ = io.WriteString(raw, "220 ready\r\n")
	r := bufio.NewReader(raw)
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "AUTH TLS") {
		s.drain(raw)
		return
	}
	_, _ = io.WriteString(raw, "234 ok\r\n")
	conn := tls.Server(raw, s.tlsConf)
	r = bufio.NewReader(conn)

	// Answer the commands of a login and of range reads.
	var data chan net.Conn
	reply := func(msg string) { _, _ = io.WriteString(conn, msg+"\r\n") }
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		cmd, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch strings.ToUpper(cmd) {
		case "USER":
			reply("331 password")
		case "PASS":
			reply("230 logged in")
		case "TYPE", "PBSZ", "PROT":
			reply("200 ok")
		case "EPSV":
			data = s.passive(reply)
		case "REST":
			reply("350 restarting")
		case "RETR":
			s.mtx.Lock()
			s.retrs++
			s.mtx.Unlock()
			reply("150 sending")
			go s.stall(data)
		case "QUIT":
			reply("221 bye")
		default:
			reply("502 not implemented")
		}
	}

	// Close at the client's close.
	s.mtx.Lock()
	s.open--
	s.closed++
	s.mtx.Unlock()
	_ = raw.Close()
}

// passive opens a data listener, announces it, and returns the channel its
// one connection arrives on.
func (s *scriptServer) passive(reply func(string)) chan net.Conn {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		reply("425 no data port")
		return nil
	}
	ch := make(chan net.Conn, 1)
	go func() {
		defer l.Close()
		conn, err := l.Accept()
		if err != nil {
			close(ch)
			return
		}
		s.opened()
		ch <- conn
	}()
	reply(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", l.Addr().(*net.TCPAddr).Port))
	return ch
}

// stall sends a few bytes on the data connection, counts the transfer under
// way, and then sends nothing until the client closes it.
func (s *scriptServer) stall(data chan net.Conn) {
	raw, ok := <-data
	if !ok || raw == nil {
		return
	}
	conn := tls.Server(raw, s.tlsConf)
	if _, err := conn.Write(make([]byte, 10)); err != nil {
		s.drain(raw)
		return
	}
	s.mtx.Lock()
	s.started++
	if s.started == 2 {
		close(s.streaming)
	}
	s.mtx.Unlock()
	s.drain(conn)
	_ = raw.Close()
}

// rangeRecorder records the errors of the range reads of an FTPS client.
type rangeRecorder struct {
	*block_store_ftps.Client
	mtx  sync.Mutex
	errs []error
}

func (c *rangeRecorder) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	data, err := c.Client.GetObjectRange(ctx, bucket, key, off, length)
	c.mtx.Lock()
	c.errs = append(c.errs, err)
	c.mtx.Unlock()
	return data, err
}

// TestRescueCloseAllOnRefusal runs at --sessions 3 with ranges against a
// server that refuses the third session with 421 while two range reads are
// mid-transfer: every session closes at once, control and data, within a
// bound; both transfers return errors; nothing is written from them while
// their bytes stay counted; and no further request goes out.
func TestRescueCloseAllOnRefusal(t *testing.T) {
	// Want two blocks of each of two packs, read by ranges.
	src := newSource(t)
	a, b := src.pack(20), src.pack(20)
	wanted := []*block.BlockRef{a[0], a[1], b[0], b[1]}
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, wanted), out)
	o.Plan.Ranges = true
	o.Sessions = 3

	// Point a real FTPS client at the server, with the tool's settings.
	srv, clientTLS := newScriptServer(t)
	client := &rangeRecorder{Client: block_store_ftps.NewClient(block_store_ftps.Config{
		Addr: srv.listener.Addr().String(), Logins: []block_store_ftps.Login{{User: "u", Password: "p"}},
		TLSConfig: clientTLS, MaxSessions: 3, ConnsPerLogin: 3, NoFullRetry: true, CloseOnDone: true,
	})}
	defer client.Close()

	// The run ends at a hard stop.
	res, err := run(context.Background(), o, client)
	var hard *hardStopError
	if !errors.As(err, &hard) {
		t.Fatalf("got %v, want a hard stop", err)
	}

	// Every socket closes within the bound of the refusal.
	const bound = 3 * time.Second
	for {
		open, accepted, closed, _, _, refusedAt := srv.counts()
		if open == 0 && accepted == closed {
			if took := time.Since(refusedAt); took > bound {
				t.Fatalf("sockets closed %v after the refusal, bound %v", took, bound)
			}
			break
		}
		if !refusedAt.IsZero() && time.Since(refusedAt) > bound {
			t.Fatalf("open %d, accepted %d, closed %d after %v", open, accepted, closed, bound)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Both transfers failed, cut by the close.
	client.mtx.Lock()
	errs := append([]error(nil), client.errs...)
	client.mtx.Unlock()
	if len(errs) != 2 {
		t.Fatalf("%d range reads, want 2: %v", len(errs), errs)
	}
	for _, err := range errs {
		if !errors.Is(err, block_store_ftps.ErrSessionsClosed) {
			t.Fatalf("range read returned %v, want ErrSessionsClosed", err)
		}
	}

	// No further request or session follows, nothing is written, and the
	// cut reads' bytes stay in the ledger's total.
	time.Sleep(300 * time.Millisecond)
	_, accepted, _, controls, retrs, _ := srv.counts()
	if controls != 3 || retrs != 2 || accepted != 5 || res.Requests != 2 {
		t.Fatalf("controls %d, retrs %d, sockets %d, requests %d after the stop", controls, retrs, accepted, res.Requests)
	}
	if res.Done != 0 || checkOutput(t, out, wanted) != 0 {
		t.Fatalf("written from cut reads: %+v", res)
	}
	if packs, _ := filepath.Glob(filepath.Join(out, "packs", "*")); len(packs) != 0 {
		t.Fatalf("output packs %v", packs)
	}
	l, err := openLedger(o.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if l.bytes != res.Bytes || res.Bytes < 2*300 || res.Bytes >= 3*300 {
		t.Fatalf("ledger total %d, result %d, want two blocks' extents", l.bytes, res.Bytes)
	}
}
