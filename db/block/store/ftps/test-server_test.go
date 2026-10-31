package block_store_ftps

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
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
}

// startTestServer starts a testServer until the test ends. It returns the
// server address and a client TLS configuration that trusts its certificate.
func startTestServer(t *testing.T) (string, *tls.Config) {
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
	}
	ftp := ftpserver.NewFtpServer(srv)
	go func() { _ = ftp.ListenAndServe() }()
	t.Cleanup(func() { _ = ftp.Stop() })
	return listener.Addr().String(), &tls.Config{RootCAs: roots}
}

// GetSettings requires TLS on the test listener.
func (s *testServer) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{Listener: s.listener, TLSRequired: ftpserver.MandatoryEncryption}, nil
}

// ClientConnected greets the client.
func (s *testServer) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "test server", nil
}

// ClientDisconnected does nothing.
func (s *testServer) ClientDisconnected(ftpserver.ClientContext) {}

// AuthUser admits user with password pass.
func (s *testServer) AuthUser(_ ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	if user != "user" || pass != "pass" {
		return nil, errors.New("bad login")
	}
	return s.fs, nil
}

// GetTLSConfig returns the server's certificate.
func (s *testServer) GetTLSConfig() (*tls.Config, error) {
	return s.tlsConf, nil
}
