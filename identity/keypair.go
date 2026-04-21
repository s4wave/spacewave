package identity

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewKeypair constructs a new keypair.
//
// authMethodID and authMethodParams can be empty.
func NewKeypair(
	pubKey crypto.PubKey,
	authMethodID string,
	authMethodParams []byte,
) (*Keypair, error) {
	pid, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	pkData, err := confparse.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	return &Keypair{
		PeerId:           pid.String(),
		PubKey:           pkData,
		AuthMethodId:     authMethodID,
		AuthMethodParams: authMethodParams,
	}, nil
}

// EntitiesToKeypairs parses all keypairs from the entities.
func EntitiesToKeypairs(ents []*Entity) ([]*Keypair, error) {
	ekps, err := EntitiesToEntityKeypairs(ents)
	if err != nil {
		return nil, err
	}
	return EntityKeypairsToKeypairs(ekps), nil
}

// NewKeypairBlock constructs a new Entity block
func NewKeypairBlock() block.Block {
	return &Keypair{}
}

// UnmarshalKeypair unmarshals a Keypair from a cursor.
// If empty, returns nil, nil
func UnmarshalKeypair(ctx context.Context, bcs *block.Cursor) (*Keypair, error) {
	return block.UnmarshalBlock[*Keypair](ctx, bcs, NewKeypairBlock)
}

// Validate validates the keypair.
func (k *Keypair) Validate() error {
	peerID, err := k.ParsePeerID()
	if err != nil {
		return err
	}
	if len(peerID) == 0 {
		return peer.ErrEmptyPeerID
	}
	pubKey, err := k.ParsePubKey()
	if err != nil {
		return err
	}
	if pubKey == nil {
		return errors.New("pub_key field cannot be empty")
	}
	if !peerID.MatchesPublicKey(pubKey) {
		pubKeyPeerID, err := peer.IDFromPublicKey(pubKey)
		if err != nil {
			return errors.Wrap(err, "pub_key")
		}
		return errors.Errorf(
			"pub_key id %s does not match peer_id %s",
			pubKeyPeerID.String(),
			peerID.String(),
		)
	}
	if k.GetAuthMethodId() == "" {
		if len(k.GetAuthMethodParams()) != 0 {
			return errors.New("auth provider params cannot be set unless auth provider id is set")
		}
	}
	return nil
}

// ParsePeerID parses the peer id field.
func (k *Keypair) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(k.GetPeerId())
}

// ParsePubKey parses the public key field.
func (k *Keypair) ParsePubKey() (crypto.PubKey, error) {
	return confparse.ParsePublicKey(k.GetPubKey())
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (k *Keypair) MarshalBlock() ([]byte, error) {
	return k.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (k *Keypair) UnmarshalBlock(data []byte) error {
	return k.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*Keypair)(nil))
