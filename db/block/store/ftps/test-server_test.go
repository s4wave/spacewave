package block_store_ftps

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

// testServer is an in-process FTP server on a memory filesystem that requires
// explicit TLS and the login user and password.
type testServer struct {
	// fs holds the files of every session.
	fs afero.Fs
	// tlsConf is the server's certificate.
	tlsConf *tls.Config
	// listener accepts control connections.
	listener net.Listener
	// mtx guards the counters below.
	mtx sync.Mutex
	// users maps each logged in session to its user.
	users map[ftpserver.ClientContext]string
	// open is the sessions now logged in per user, and peak the most at once.
	open, peak map[string]int
	// total is the sessions logged in now and totalPeak the most at once.
	total, totalPeak int
	// logins counts the logins per user.
	logins map[string]int
	// spendPartial ends the session of a client that closes a download
	// before its end, as ProFTPD does.
	spendPartial bool
	// dialDelay delays each greeting, and readDelay each download read.
	dialDelay, readDelay time.Duration
}

// startTestServer starts a testServer until the test ends. It returns the
// server address and a client TLS configuration that trusts its certificate.
func startTestServer(t *testing.T) (string, *tls.Config) {
	addr, tlsConf, _ := startCountingServer(t)
	return addr, tlsConf
}

// startCountingServer starts a testServer and returns it for its counters.
func startCountingServer(t *testing.T) (string, *tls.Config, *testServer) {
	t.Helper()

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

	// Serve the memory filesystem until the test ends.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &testServer{
		fs:       afero.NewMemMapFs(),
		tlsConf:  &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		listener: listener,
		users:    make(map[ftpserver.ClientContext]string),
		open:     make(map[string]int),
		peak:     make(map[string]int),
		logins:   make(map[string]int),
	}
	ftp := ftpserver.NewFtpServer(srv)
	go func() { _ = ftp.ListenAndServe() }()
	t.Cleanup(func() { _ = ftp.Stop() })
	return listener.Addr().String(), &tls.Config{RootCAs: roots}, srv
}

// GetSettings requires TLS on the test listener.
func (s *testServer) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{Listener: s.listener, TLSRequired: ftpserver.MandatoryEncryption}, nil
}

// ClientConnected greets the client.
func (s *testServer) ClientConnected(ftpserver.ClientContext) (string, error) {
	s.mtx.Lock()
	delay := s.dialDelay
	s.mtx.Unlock()
	time.Sleep(delay)
	return "test server", nil
}

// ClientDisconnected stops counting the session.
func (s *testServer) ClientDisconnected(cc ftpserver.ClientContext) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if user, ok := s.users[cc]; ok {
		s.open[user]--
		s.total--
		delete(s.users, cc)
	}
}

// AuthUser admits user1, user2 and user with password pass, counting their
// sessions.
func (s *testServer) AuthUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	if (user != "user" && user != "user1" && user != "user2") || pass != "pass" {
		return nil, errors.New("bad login")
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.users[cc] = user
	s.logins[user]++
	s.open[user]++
	s.peak[user] = max(s.peak[user], s.open[user])
	s.total++
	s.totalPeak = max(s.totalPeak, s.total)
	return &spendFs{Fs: s.fs, srv: s, cc: cc}, nil
}

// spendFs opens files that end the client's session when closed before their
// end, when the server spends partial downloads.
type spendFs struct {
	afero.Fs
	srv *testServer
	cc  ftpserver.ClientContext
}

// Open opens a file that ends the session on a partial read.
func (f *spendFs) Open(name string) (afero.File, error) {
	file, err := f.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &spendFile{File: file, fs: f}, nil
}

// OpenFile opens a file for reading like Open, or as the filesystem does.
func (f *spendFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := f.Fs.OpenFile(name, flag, perm)
	if err != nil || flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return file, err
	}
	return &spendFile{File: file, fs: f}, nil
}

// spendFile ends its client's session when closed before reading to its end.
type spendFile struct {
	afero.File
	fs  *spendFs
	eof bool
}

func (f *spendFile) Read(p []byte) (int, error) {
	f.fs.srv.mtx.Lock()
	delay := f.fs.srv.readDelay
	f.fs.srv.mtx.Unlock()
	time.Sleep(delay)
	n, err := f.File.Read(p)
	if err == io.EOF {
		f.eof = true
	}
	return n, err
}

func (f *spendFile) Close() error {
	err := f.File.Close()
	f.fs.srv.mtx.Lock()
	spend := f.fs.srv.spendPartial && !f.eof
	f.fs.srv.mtx.Unlock()
	if spend {
		_ = f.fs.cc.Close()
	}
	return err
}

// GetTLSConfig returns the server's certificate.
func (s *testServer) GetTLSConfig() (*tls.Config, error) {
	return s.tlsConf, nil
}
