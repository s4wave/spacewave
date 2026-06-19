package p2ptls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"net"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func TestIdentityHandshakeWithEd25519CertificateKey(t *testing.T) {
	clientPriv := generateHostKey(t)
	serverPriv := generateHostKey(t)
	clientID := peerIDFromPrivateKey(t, clientPriv)
	serverID := peerIDFromPrivateKey(t, serverPriv)
	clientIdentity := newIdentity(t, clientPriv)
	serverIdentity := newIdentity(t, serverPriv)

	assertEd25519CertificateKey(t, clientIdentity)
	assertEd25519CertificateKey(t, serverIdentity)

	clientConf, serverKeyCh := clientIdentity.ConfigForPeer(serverID)
	serverConf, clientKeyCh := serverIdentity.ConfigForPeer(clientID)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientTLS := tls.Client(clientConn, clientConf)
	defer clientTLS.Close()
	serverTLS := tls.Server(serverConn, serverConf)
	defer serverTLS.Close()

	errCh := make(chan error, 2)
	go func() {
		errCh <- serverTLS.Handshake()
	}()
	go func() {
		errCh <- clientTLS.Handshake()
	}()
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	assertPeerKey(t, serverKeyCh, serverPriv.GetPublic(), "server")
	assertPeerKey(t, clientKeyCh, clientPriv.GetPublic(), "client")
}

func generateHostKey(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func peerIDFromPrivateKey(t *testing.T, priv crypto.PrivKey) peer.ID {
	t.Helper()
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newIdentity(t *testing.T, priv crypto.PrivKey) *Identity {
	t.Helper()
	identity, err := NewIdentity(priv)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func assertEd25519CertificateKey(t *testing.T, identity *Identity) {
	t.Helper()
	if len(identity.config.Certificates) != 1 {
		t.Fatalf("certificate count = %d", len(identity.config.Certificates))
	}
	if _, ok := identity.config.Certificates[0].PrivateKey.(ed25519.PrivateKey); !ok {
		t.Fatalf("certificate private key type = %T", identity.config.Certificates[0].PrivateKey)
	}
}

func assertPeerKey(t *testing.T, ch <-chan crypto.PubKey, want crypto.PubKey, label string) {
	t.Helper()
	got, ok := <-ch
	if !ok {
		t.Fatalf("%s peer key channel closed without a key", label)
	}
	if !got.Equals(want) {
		t.Fatalf("%s peer key did not match the expected host key", label)
	}
}
