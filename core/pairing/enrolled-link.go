package pairing

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// BindEnrolledLink authenticates the receiving Session on an existing encrypted
// pairing connection. The receiving key's signed proof binds it to both original
// transport peers and this exact account enrollment. Bilateral account approval
// and durable enrollment must precede handing the result to ordinary routing.
func BindEnrolledLink(connection link.Link, offer *AccountOffer, identity *Identity, source, receiving peer.ID) (link.Link, error) {
	// Validate the enrollment proof and bind it to this connection's peers.
	if err := ValidateIdentity(offer, identity, source, receiving); err != nil {
		return nil, err
	}
	local, remote := connection.GetLocalPeer(), connection.GetRemotePeer()
	if (local != source || remote != receiving) && (local != receiving || remote != source) {
		return nil, errors.New("enrollment proof belongs to another authenticated connection")
	}
	enrolled, _, err := peer.ParsePeerIDWithPubKey(identity.GetSessionProof().GetResponderPeerId())
	if err != nil {
		return nil, err
	}
	if local == receiving {
		local = enrolled
	} else {
		remote = enrolled
	}
	return &enrolledLink{connection: connection, local: local, remote: remote}, nil
}

// enrolledLink retains the connection's encryption, streams, and lifetime while
// routing the independently proved Session identity instead of its setup peer.
type enrolledLink struct {
	connection    link.Link
	local, remote peer.ID
}

func (l *enrolledLink) GetLocalPeer() peer.ID  { return l.local }
func (l *enrolledLink) GetRemotePeer() peer.ID { return l.remote }

func (l *enrolledLink) GetUUID() uint64                { return l.connection.GetUUID() }
func (l *enrolledLink) GetTransportUUID() uint64       { return l.connection.GetTransportUUID() }
func (l *enrolledLink) GetRemoteTransportUUID() uint64 { return l.connection.GetRemoteTransportUUID() }
func (l *enrolledLink) Close() error                   { return l.connection.Close() }

func (l *enrolledLink) OpenStream(opts stream.OpenOpts) (stream.Stream, error) {
	return l.connection.OpenStream(opts)
}

func (l *enrolledLink) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	return l.connection.AcceptStream()
}

var _ link.Link = (*enrolledLink)(nil)
