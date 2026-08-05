package pubmessage

import (
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

const pubMessageEncContext = "bifrost/pubsub/pubmessage 2024-06-05T02:38:47.55258Z channel/"

// NewPubMessage constructs/signs/encodes a new pub-message and inner message.
//
// Uses time.Now() for the timestamp: not deterministic.
func NewPubMessage(
	channelID string,
	privKey crypto.PrivKey,
	hashType hash.HashType,
	data []byte,
) (*peer.SignedMsg, *PubMessageInner, error) {
	// Build and serialize the signed inner message.
	inner := &PubMessageInner{
		Data:      data,
		Channel:   channelID,
		Timestamp: timestamp.Now(),
	}
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, inner, err
	}

	// Sign the serialized payload with the channel encryption context.
	sig, err := peer.NewSignedMsg(pubMessageEncContext+channelID, privKey, hashType, innerData)
	return sig, inner, err
}

// ExtractAndVerify extracts the inner message from a signed message.
func ExtractAndVerify(msg *peer.SignedMsg) (*PubMessageInner, crypto.PubKey, peer.ID, error) {
	// Decode and validate the signed inner message before verifying identity.
	data := msg.GetData()

	out := &PubMessageInner{}
	err := out.UnmarshalVT(data)
	if err == nil {
		err = out.Validate()
	}
	if err != nil {
		return nil, nil, "", err
	}

	// Verify the signature against the decoded channel context.
	pubKey, peerID, err := msg.ExtractAndVerify(pubMessageEncContext + out.GetChannel())
	if err != nil {
		return nil, nil, "", err
	}
	return out, pubKey, peerID, nil
}
