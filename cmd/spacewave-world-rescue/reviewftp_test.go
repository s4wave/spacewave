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
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
)

// miniFTPS is a small explicit-TLS FTP server with scripted misbehaviour. It
// counts its sessions, logins and sockets; a session stops counting just
// before its socket closes, closeLag after its control connection ends.
type miniFTPS struct {
	t        *testing.T
	ln       net.Listener
	tlsConf  *tls.Config
	cliTLS   *tls.Config
	data     []byte
	userResp string // reply to USER, default 331
	// onRetr runs a RETR: ctl writes replies, dc is the data conn.
	onRetr func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64)
	// closeLag keeps a session counted that long after its control ends.
	// closeLag delays the close of a session whose control connection ended,
	// as a server slow to drop a session.
	closeLag time.Duration
	// greetDelay delays the greeting of the nth connection.
	greetDelay func(n int) time.Duration
	conns      int
	loginAt    []time.Time

	mtx                       sync.Mutex
	open, peak, logins, users int
	sockets, peakSockets      int
	cmds                      []string
	retr421                   *time.Time
}

// newMini starts a miniFTPS until the test ends.
func newMini(t *testing.T) *miniFTPS {
	// Issue a self-signed certificate for 127.0.0.1.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}}
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

	// Serve each connection until the test ends.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &miniFTPS{t: t, ln: ln, data: make([]byte, 64<<10),
		tlsConf: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		cliTLS:  &tls.Config{RootCAs: roots}}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go m.serve(c)
		}
	}()
	return m
}

// sock counts d sockets opened.
func (m *miniFTPS) sock(d int) {
	m.mtx.Lock()
	m.sockets += d
	m.peakSockets = max(m.peakSockets, m.sockets)
	m.mtx.Unlock()
}

// log records a command.
func (m *miniFTPS) log(s string) {
	m.mtx.Lock()
	m.cmds = append(m.cmds, s)
	m.mtx.Unlock()
}

// serve answers one control connection.
func (m *miniFTPS) serve(raw net.Conn) {
	// Count the session open until just before its socket closes.
	m.sock(1)
	m.mtx.Lock()
	m.open++
	m.peak = max(m.peak, m.open)
	m.mtx.Unlock()
	defer func() {
		time.Sleep(m.closeLag)
		m.mtx.Lock()
		m.open--
		m.mtx.Unlock()
		m.sock(-1)
		_ = raw.Close()
	}()

	// Greet, then answer commands.
	conn := raw
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	reply := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }
	m.mtx.Lock()
	m.conns++
	cn := m.conns
	m.mtx.Unlock()
	if m.greetDelay != nil {
		time.Sleep(m.greetDelay(cn))
	}
	reply("220 mini")
	var dl net.Listener
	var rest int64
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		cmd, arg, _ := strings.Cut(line, " ")
		m.log(cmd)
		switch strings.ToUpper(cmd) {
		case "AUTH":
			reply("234 ok")
			tc := tls.Server(raw, m.tlsConf)
			if tc.Handshake() != nil {
				return
			}
			conn = tc
			r, w = bufio.NewReader(conn), bufio.NewWriter(conn)
		case "USER":
			m.mtx.Lock()
			m.users++
			m.mtx.Unlock()
			if m.userResp != "" {
				reply(m.userResp)
				return
			}
			reply("331 pass")
		case "PASS":
			m.mtx.Lock()
			m.logins++
			m.loginAt = append(m.loginAt, time.Now())
			m.mtx.Unlock()
			reply("230 in")
		case "FEAT":
			reply("502 no")
		case "TYPE", "PBSZ", "PROT":
			reply("200 ok")
		case "EPSV":
			dl, _ = net.Listen("tcp", "127.0.0.1:0")
			reply(fmt.Sprintf("229 ok (|||%d|)", dl.Addr().(*net.TCPAddr).Port))
		case "REST":
			_, _ = fmt.Sscan(arg, &rest)
			reply("350 ok")
		case "SIZE":
			reply(fmt.Sprintf("213 %d", len(m.data)))
		case "RETR":
			if m.retr421 != nil {
				m.mtx.Lock()
				*m.retr421 = time.Now()
				m.mtx.Unlock()
				reply("421 Too many connections")
				return
			}
			reply("150 go")
			dc, err := dl.Accept()
			_ = dl.Close()
			if err != nil {
				return
			}
			m.sock(1)
			tdc := tls.Server(dc, m.tlsConf)
			m.onRetr(m, w, raw, tdc, rest)
			_ = dc.Close()
			m.sock(-1)
			rest = 0
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 " + cmd)
		}
	}
}

// counts returns the peak sessions, the logins, the USER commands and the
// peak sockets.
func (m *miniFTPS) counts() (peak, logins, users, peakSockets int) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.peak, m.logins, m.users, m.peakSockets
}

// client returns an FTPS client of the server with the tool's settings,
// closed at the test's end.
func (m *miniFTPS) client(t *testing.T, sessions int, shut time.Duration) *block_store_ftps.Client {
	c := block_store_ftps.NewClient(block_store_ftps.Config{
		Addr: m.ln.Addr().String(), TLSConfig: m.cliTLS,
		Logins:      []block_store_ftps.Login{{User: "u", Password: "p"}},
		MaxSessions: sessions, ConnsPerLogin: sessions, NoFullRetry: true, CloseOnDone: true,
		ShutTimeout: shut,
	})
	t.Cleanup(c.Close)
	return c
}

// okRetr sends the object from rest and ends the transfer with 226.
func okRetr(m *miniFTPS, ctl *bufio.Writer, _ net.Conn, dc net.Conn, rest int64) {
	_, _ = dc.Write(m.data[rest:])
	_ = dc.Close()
	_, _ = ctl.WriteString("226 done\r\n")
	_ = ctl.Flush()
}

// reviewFetcher returns a fetcher of c with retries.
func reviewFetcher(c objectReader) *fetcher {
	return newFetcher(c, "b", fetchOptions{Timeout: 3 * time.Second, Retries: 3, Backoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, CapBytes: 1 << 40}, 0)
}

// TestReviewUser530 answers USER with each refusal a server may send: the
// first is a hard stop, with no second login.
func TestReviewUser530(t *testing.T) {
	for _, refusal := range []string{
		"530 Login incorrect.",
		"421 Service not available, closing control connection.",
		"421 10 users (the maximum) are already logged in, sorry",
	} {
		t.Run(refusal[:3], func(t *testing.T) {
			// Refuse every login.
			m := newMini(t)
			m.onRetr = okRetr
			m.userResp = refusal
			f := reviewFetcher(m.client(t, 1, 0))

			// The first refusal stops the run.
			_, err := f.whole(context.Background(), "k", uint64(len(m.data)))
			var hard *hardStopError
			if !errors.As(err, &hard) {
				t.Fatalf("%q: got %v, want a hard stop", refusal, err)
			}
			if _, _, users, _ := m.counts(); users != 1 {
				t.Fatalf("%q: %d logins tried", refusal, users)
			}
		})
	}
}

// TestReviewMid421 sends 421 and resets the data connection mid-transfer:
// the read is a hard stop, with no redial.
func TestReviewMid421(t *testing.T) {
	// Send part of the object, then 421, then reset.
	m := newMini(t)
	m.onRetr = func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, _ int64) {
		_, _ = dc.Write(m.data[:1000])
		_, _ = ctl.WriteString("421 Too many connections, closing control connection\r\n")
		_ = ctl.Flush()
		rstData(dc)
		time.Sleep(50 * time.Millisecond)
		_ = raw.Close()
	}
	f := reviewFetcher(m.client(t, 1, 0))

	// The 421 stops the run after one login and one request.
	_, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	var hard *hardStopError
	if !errors.As(err, &hard) {
		t.Fatalf("got %v, want a hard stop", err)
	}
	if _, logins, _, _ := m.counts(); logins != 1 || f.requests.Load() != 1 {
		t.Fatalf("%d logins, %d requests", logins, f.requests.Load())
	}
}

// TestReviewRange421 answers the early close of a range read with 421: the
// read is a hard stop, and no later read goes out.
func TestReviewRange421(t *testing.T) {
	// Answer the first transfer's close with 421.
	m := newMini(t)
	var n int
	m.onRetr = func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
		n++
		_, _ = dc.Write(m.data[rest:])
		_ = dc.Close()
		if n == 1 {
			_, _ = ctl.WriteString("421 Too many connections, closing control connection\r\n")
			_ = ctl.Flush()
			_ = raw.Close()
			return
		}
		_, _ = ctl.WriteString("226 done\r\n")
		_ = ctl.Flush()
	}
	f := reviewFetcher(m.client(t, 1, 0))

	// The first read stops the run and the second is refused.
	_, err1 := f.rangeRead(context.Background(), "k", 0, 100)
	_, err2 := f.rangeRead(context.Background(), "k", 100, 100)
	var hard *hardStopError
	if !errors.As(err1, &hard) || !errors.Is(err2, errHardStopped) {
		t.Fatalf("first read %v, second %v", err1, err2)
	}
	if _, logins, _, _ := m.counts(); logins != 1 {
		t.Fatalf("%d logins", logins)
	}
}

// TestReviewRedialOverlap resets the data connection mid-transfer, keeping
// the control connection: the client closes the session and redials only
// once the server closed it, so the server never holds two sessions under
// --sessions 1.
func TestReviewRedialOverlap(t *testing.T) {
	// Reset the first transfer, serve the second, and drop a session slowly.
	m := newMini(t)
	m.closeLag = 200 * time.Millisecond
	var n int
	m.onRetr = func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
		n++
		if n == 1 {
			_, _ = dc.Write(m.data[:1000])
			rstData(dc)
			_, _ = ctl.WriteString("451 aborted\r\n")
			_ = ctl.Flush()
			return
		}
		okRetr(m, ctl, raw, dc, rest)
	}
	f := reviewFetcher(m.client(t, 1, 0))

	// The retry succeeds on a second session that never overlapped the first.
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	if err != nil || len(data) != len(m.data) {
		t.Fatalf("%v, %d bytes", err, len(data))
	}
	if peak, logins, _, _ := m.counts(); peak != 1 || logins != 2 {
		t.Fatalf("peak sessions %d under --sessions 1, logins %d", peak, logins)
	}
}

// rstData resets the raw data socket with no TLS close_notify.
func rstData(dc net.Conn) {
	tc := dc.(*tls.Conn)
	time.Sleep(20 * time.Millisecond)
	tcp := tc.NetConn().(*net.TCPConn)
	_ = tcp.SetLinger(0)
	_ = tcp.Close()
}

// TestReviewSpareAfterHardStop answers a range read's RETR with 421 while
// the spare it dialed is still greeting: the spare never logs in.
func TestReviewSpareAfterHardStop(t *testing.T) {
	// Delay the spare's greeting past the 421.
	m := newMini(t)
	m.greetDelay = func(n int) time.Duration {
		if n == 2 {
			return 150 * time.Millisecond
		}
		return 0
	}
	var stopAt time.Time
	m.retr421 = &stopAt
	f := reviewFetcher(m.client(t, 2, 0))

	// The read stops the run.
	_, err := f.rangeRead(context.Background(), "k", 0, 100)
	var hard *hardStopError
	if !errors.As(err, &hard) {
		t.Fatalf("got %v, want a hard stop", err)
	}

	// No login follows the 421.
	time.Sleep(500 * time.Millisecond)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var after int
	for _, at := range m.loginAt {
		if at.After(stopAt) {
			after++
		}
	}
	if after != 0 {
		t.Fatalf("%d logins after the 421", after)
	}
}

// TestReviewStaleDeadline idles a pooled session past the shut timeout
// after a transfer: the deadline the FTP library set at the transfer's end
// is cleared, so the next read reuses the session.
func TestReviewStaleDeadline(t *testing.T) {
	// Read once with a short shut timeout.
	m := newMini(t)
	m.onRetr = okRetr
	f := reviewFetcher(m.client(t, 1, 300*time.Millisecond))
	if _, err := f.whole(context.Background(), "k", uint64(len(m.data))); err != nil {
		t.Fatal(err)
	}

	// Idle past it and read again on the same session.
	time.Sleep(600 * time.Millisecond)
	if _, err := f.whole(context.Background(), "k", uint64(len(m.data))); err != nil {
		t.Fatal(err)
	}
	if _, logins, _, _ := m.counts(); logins != 1 {
		t.Fatalf("idle pooled session died at the stale deadline: %d logins", logins)
	}
}
