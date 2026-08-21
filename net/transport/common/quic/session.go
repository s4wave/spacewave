package transport_quic

import (
	"context"
	"errors"
	"net"

	"github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Alpn is set to ensure quic does not talk to non-bifrost peers
const Alpn = "bifrost"

// DialSession dials a remote addr on a packet conn to create a session.
//
// Negotiates a TLS session. Specify a empty peer ID to allow any.
// Dial indicates this is the originator of the conn.
func DialSession(
	ctx context.Context,
	le *logrus.Entry,
	opts *Opts,
	pconn net.PacketConn,
	identity *p2ptls.Identity,
	addr net.Addr,
	rpeer peer.ID,
) (*quic.Conn, crypto.PubKey, error) {
	// Build the peer-specific TLS configuration and QUIC settings.
	tlsConf, keyCh := identity.ConfigForPeer(rpeer)
	tlsConf.NextProtos = []string{Alpn}
	quicConfig := BuildQuicConfig(opts)

	// Dial the QUIC session over the packet connection.
	sess, err := quic.Dial(ctx, pconn, addr, tlsConf, quicConfig)
	if err != nil {
		return nil, nil, err
	}

	// Wait for the authenticated remote public key.
	var remotePubKey crypto.PubKey
	select {
	case remotePubKey = <-keyCh:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	// Require the TLS verifier to provide the remote public key.
	if remotePubKey == nil {
		return nil, nil, errors.New("expected remote pub key to be set")
	}

	return sess, remotePubKey, nil
}

// DialSessionViaTransport dials a remote addr on a quic transport to create a session.
//
// Negotiates a TLS session. Specify a empty peer ID to allow any.
// Dial indicates this is the originator of the conn.
func DialSessionViaTransport(
	ctx context.Context,
	le *logrus.Entry,
	opts *Opts,
	tpt *quic.Transport,
	identity *p2ptls.Identity,
	addr net.Addr,
	rpeer peer.ID,
) (*quic.Conn, crypto.PubKey, error) {
	// Build the peer-specific TLS configuration and QUIC settings.
	tlsConf, keyCh := identity.ConfigForPeer(rpeer)
	tlsConf.NextProtos = []string{Alpn}
	quicConfig := BuildQuicConfig(opts)

	// Dial the QUIC session through the shared transport.
	sess, err := tpt.Dial(ctx, addr, tlsConf, quicConfig)
	if err != nil {
		return nil, nil, err
	}

	// Wait for the authenticated remote public key.
	var remotePubKey crypto.PubKey
	select {
	case remotePubKey = <-keyCh:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	// Require the TLS verifier to provide the remote public key.
	if remotePubKey == nil {
		return nil, nil, errors.New("expected remote pub key to be set")
	}

	return sess, remotePubKey, nil
}

// ListenSession listens for a single incoming session on a PacketConn.
//
// Negotiates a TLS session. Specify a empty peer ID to allow any.
func ListenSession(
	ctx context.Context,
	le *logrus.Entry,
	opts *Opts,
	pconn net.PacketConn,
	identity *p2ptls.Identity,
	rpeer peer.ID,
) (*quic.Conn, error) {
	// Build the listener configuration and TLS verifier.
	quicConfig := BuildQuicConfig(opts)
	tlsConf := BuildIncomingTlsConf(identity, rpeer)

	// Start listening for the incoming QUIC handshake.
	le.Debug("listening for incoming handshake with quic + tls")
	ln, err := quic.Listen(pconn, tlsConf, quicConfig)
	if err != nil {
		return nil, err
	}

	// Accept one session and close the listener on failure.
	sess, err := ln.Accept(ctx)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}

	return sess, nil
}

// DetermineSessionIdentity determines the identity from the session cert chain.
func DetermineSessionIdentity(sess *quic.Conn) (peer.ID, crypto.PubKey, error) {
	// Extract the remote public key from the session certificate chain.
	connState := sess.ConnectionState()
	certs := connState.TLS.PeerCertificates
	remotePubKey, err := p2ptls.PubKeyFromCertChain(certs)
	if err != nil {
		return "", nil, err
	}

	// Derive the remote peer identity from its public key.
	remotePeerID, err := peer.IDFromPublicKey(remotePubKey)
	if err != nil {
		return "", nil, err
	}
	return remotePeerID, remotePubKey, nil
}
