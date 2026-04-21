package transport_quic

import (
	"context"
	"crypto"
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
	remotePeerID, remotePubKey, err := DetermineSessionIdentity(sess)
	if err != nil {
		return nil, err
	}

	remoteAddr := sess.RemoteAddr()
	nctx, nctxCancel := context.WithCancel(ctx)
	uuid := NewLinkUUID(localAddr, remoteAddr, remotePeerID)
	remoteTransportUUID := NewTransportUUID(remoteAddr.String(), remotePeerID)
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

// GetRemotePeerPubKey returns the remote peer public key
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

// OpenStream opens a stream on the link, with the given parameters.
func (l *Link) OpenStream(opts stream.OpenOpts) (stream.Stream, error) {
	// OpenStream returns an error if we hit the stream limit.
	// it is better to return an error and backoff / know something is wrong,
	// than wait forever (potentially) while we are at the cap.
	return l.sess.OpenStream()
}

// AcceptStream accepts a stream from the link.
func (l *Link) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	qstream, err := l.sess.AcceptStream(l.ctx)
	if l.ctx.Err() != nil {
		// detect link shutdown, avoid logging unnecessary errors
		if qstream != nil {
			_ = qstream.Close()
		}
		return nil, stream.OpenOpts{}, context.Canceled
	}
	if err != nil {
		qe, qeOk := err.(*quic.ApplicationError)
		if qeOk && qe != nil {
			// remote shutdown of connection normally
			if qe.ErrorCode == 0 {
				err = io.EOF
			}
		}

		return nil, stream.OpenOpts{}, err
	}

	opts := stream.OpenOpts{}
	return qstream, opts, nil
}

// Close closes the connection.
func (l *Link) Close() error {
	l.closedOnce.Do(func() {
		l.ctxCancel()
		if closed := l.closed; closed != nil {
			closed()
		}
		if l.sess != nil {
			_ = l.sess.CloseWithError(0, "")
		}
	})
	return nil
}

// _ is a type assertion
var _ link.Link = ((*Link)(nil))
