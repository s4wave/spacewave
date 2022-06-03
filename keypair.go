package identity

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/block"
	proto "google.golang.org/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/pkg/errors"
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
		PeerId:           pid.Pretty(),
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
func UnmarshalKeypair(bcs *block.Cursor) (*Keypair, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewKeypairBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*Keypair)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate validates the keypair.
func (k *Keypair) Validate() error {
	peerID, err := k.ParsePeerID()
	if err != nil {
		return err
	}
	if len(peerID) == 0 {
		return peer.ErrPeerIDEmpty
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
			pubKeyPeerID.Pretty(),
			peerID.Pretty(),
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
	return proto.Marshal(k)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (k *Keypair) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, k)
}

// _ is a type assertion
var _ block.Block = ((*Keypair)(nil))
