package signaling_rpc

import (
	"errors"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ProtocolID is the default protocol ID for the signaling server.
var ProtocolID = protocol.ID("bifrost/signaling")

// encContext is the encryption context used for the signaling session messages.
const encContext = "bifrost/signaling/rpc session msg 2024-06-05T02:45:07.208906Z"

// NewSessionMsg creates a new SessionMsg with the provided data signed.
func NewSessionMsg(privKey crypto.PrivKey, hashType hash.HashType, msg []byte, seqno uint64) (*SessionMsg, error) {
	signedMsg, err := peer.NewSignedMsg(encContext, privKey, hashType, msg)
	if err != nil {
		return nil, err
	}
	return &SessionMsg{
		SignedMsg: signedMsg,
		Seqno:     seqno,
	}, nil
}

// Validate validates the Listen request.
func (r *ListenRequest) Validate() error {
	return nil
}

// Validate validates the Session request.
func (r *SessionRequest) Validate() error {
	switch b := r.GetBody().(type) {
	case *SessionRequest_Init:
		if err := b.Init.Validate(); err != nil {
			return err
		}
	case *SessionRequest_AckMsg:
	case *SessionRequest_ClearMsg:
	case *SessionRequest_SendMsg:
		// verify the signed message & the signature.
		if err := b.SendMsg.Validate(); err != nil {
			return err
		}
	default:
		return errors.New("unknown session request message type")
	}
	return nil
}

// Validate validates the Session response.
//
// NOTE: this checks the signature but not which peer the signature is from.
// Be sure to additionally check the origin of the signature is what you expect.
func (r *SessionResponse) Validate() error {
	switch b := r.GetBody().(type) {
	case *SessionResponse_Opened:
	case *SessionResponse_Closed:
	case *SessionResponse_ClearMsg:
	case *SessionResponse_AckMsg:
	case *SessionResponse_RecvMsg:
		// verify the signed message & the signature.
		if err := b.RecvMsg.Validate(); err != nil {
			return err
		}
	default:
		return errors.New("unknown session response message type")
	}
	return nil
}

// Validate validates the SessionMsg.
func (m *SessionMsg) Validate() error {
	_, _, err := m.GetSignedMsg().ExtractAndVerify(encContext)
	return err
}

// Validate validates the SessionInit.
func (i *SessionInit) Validate() error {
	pid, err := i.ParsePeerID()
	if err != nil {
		return err
	}
	if len(pid) == 0 {
		return peer.ErrEmptyPeerID
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (i *SessionInit) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(i.GetPeerId())
}

// ExtractAndVerify extracts the signed message and verifies the signature.
func (m *SessionMsg) ExtractAndVerify() (crypto.PubKey, peer.ID, error) {
	return m.GetSignedMsg().ExtractAndVerify(encContext)
}
