package transport_quic

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"

	quic "github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

// Link represents a Quic-based connection/link.
type Link struct {
	// ctx is the context for this link
	ctx context.Context
	// ctxCancel cancels the context for this link.
	ctxCancel context.CancelFunc
	// le is the log entry
	le *logrus.Entry
	// addr is the bound remote address
	addr net.Addr
	// localAddr is the local address
	localAddr net.Addr
	// sess is the quic session
	sess *quic.Conn
	// remotePeerID is the remote peer id
	remotePeerID peer.ID
	// remotePubKey is the remote public key
	remotePubKey crypto.PublicKey
	// uuid is the link uuid
	uuid uint64
	// transportUUID is the transport uuid
	transportUUID uint64
	// remoteTransportUUID is the remote transport uuid
	remoteTransportUUID uint64
	// localPeerID is the local peer ID
	localPeerID peer.ID
	// closed is the closed callback
	closed func()
	// closedOnce guards closed
	closedOnce sync.Once

	// messagesMtx guards messages.
	messagesMtx sync.Mutex
	// messages holds the open message streams by datagram prefix.
	messages map[string]*messageStream
	// receiveOnce starts the datagram receiver with the first message stream.
	receiveOnce sync.Once
}

// NewLink builds a new link.
func NewLink(
	ctx context.Context,
	le *logrus.Entry,
	opts *Opts,
	localTransportUUID uint64,
	localPeerID peer.ID,
	localAddr net.Addr,
	sess *quic.Conn,
	closed func(),
) (*Link, error) {
	// Determine the remote identity from the negotiated session.
	remotePeerID, remotePubKey, err := DetermineSessionIdentity(sess)
	if err != nil {
		return nil, err
	}

	// Each physical connection has its own identity. Manual and signaled
	// WebRTC connections can share both addresses and authenticated peers.
	remoteAddr := sess.RemoteAddr()
	var linkID [8]byte
	if _, err := rand.Read(linkID[:]); err != nil {
		return nil, err
	}

	nctx, nctxCancel := context.WithCancel(ctx) //nolint:gosec // cancel stored on Link and called by Close
	uuid := binary.LittleEndian.Uint64(linkID[:])
	remoteTransportUUID := NewTransportUUID(remoteAddr.String(), remotePeerID)

	// Assemble the link state with local and remote session metadata.
	return &Link{
		ctx:       nctx,
		ctxCancel: nctxCancel,

		le:                  le.WithField("link-uuid", uuid),
		addr:                remoteAddr,
		uuid:                uuid,
		sess:                sess,
		remotePeerID:        remotePeerID,
		remotePubKey:        remotePubKey,
		closed:              closed,
		localAddr:           localAddr,
		localPeerID:         localPeerID,
		transportUUID:       localTransportUUID,
		remoteTransportUUID: remoteTransportUUID,
		messages:            make(map[string]*messageStream),
	}, nil
}

// GetContext returns a context that is canceled when the Link is closed.
func (l *Link) GetContext() context.Context {
	return l.ctx
}

// GetUUID returns the link unique id.
func (l *Link) GetUUID() uint64 {
	return l.uuid
}

// GetTransportUUID returns the unique ID of the transport.
func (l *Link) GetTransportUUID() uint64 {
	return l.transportUUID
}

// GetRemoteTransportUUID returns the unique ID of the remote transport.
// Reported by the remote peer. May be zero or unknown value.
func (l *Link) GetRemoteTransportUUID() uint64 {
	return l.remoteTransportUUID
}

// GetRemotePeer returns the identity of the remote peer if encrypted.
func (l *Link) GetRemotePeer() peer.ID {
	return l.remotePeerID
}

// GetRemotePeerPubKey returns the remote peer public key.
func (l *Link) GetRemotePeerPubKey() crypto.PublicKey {
	return l.remotePubKey
}

// GetLocalPeer returns the identity of the local peer.
func (l *Link) GetLocalPeer() peer.ID {
	return l.localPeerID
}

// LocalAddr returns the local address.
func (l *Link) LocalAddr() net.Addr {
	return l.localAddr
}

// RemoteAddr returns the remote address.
func (l *Link) RemoteAddr() net.Addr {
	return l.addr
}

// OpenStream opens a stream on the link, with the given parameters. An
// unreliable stream is a MessageStream whose control is the opened stream.
func (l *Link) OpenStream(opts stream.OpenOpts) (stream.Stream, error) {
	// OpenStream returns an error if we hit the stream limit.
	// it is better to return an error and backoff / know something is wrong,
	// than wait forever (potentially) while we are at the cap.
	qstream, err := l.sess.OpenStream()
	if err != nil {
		return nil, err
	}
	if !opts.Unreliable {
		return reliableStream{qstream}, nil
	}
	msgs, err := l.newMessageStream(qstream)
	if err != nil {
		qstream.CancelRead(0)
		_ = qstream.Close()
		return nil, err
	}
	return msgs, nil
}

// AcceptStream accepts a stream from the link.
func (l *Link) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	// Accept the next QUIC stream and handle link cancellation.
	qstream, err := l.sess.AcceptStream(l.ctx)
	if l.ctx.Err() != nil {
		// Close a partially accepted stream after link shutdown.
		if qstream != nil {
			_ = reliableStream{qstream}.Close()
		}
		return nil, stream.OpenOpts{}, context.Canceled
	}

	// Translate clean QUIC closure to EOF and preserve other errors.
	if err != nil {
		if isCleanAcceptClose(err) {
			return nil, stream.OpenOpts{}, io.EOF
		}
		return nil, stream.OpenOpts{}, err
	}

	// Return the accepted stream with default open options.
	return reliableStream{qstream}, stream.OpenOpts{}, nil
}

// Close closes the connection.
func (l *Link) Close() error {
	l.closedOnce.Do(func() {
		// Cancel the link context before invoking the close callback.
		l.ctxCancel()
		if closed := l.closed; closed != nil {
			closed()
		}

		// Close the QUIC session after invoking the close callback.
		if l.sess != nil {
			_ = l.sess.CloseWithError(0, "")
		}
	})
	return nil
}

// _ is a type assertion
var _ link.MessageLink = (*Link)(nil)
