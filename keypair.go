package identity

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/block"
	proto "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/pkg/errors"
)

// NewKeypair constructs a new keypair.
func NewKeypair(entityID, domainID string, pubKey crypto.PubKey) (*Keypair, error) {
	pid, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	/*
		pkData, err := crypto.MarshalPublicKey(pubKey)
		if err != nil {
			return nil, err
		}
	*/
	return &Keypair{
		EntityId: entityID,
		DomainId: domainID,
		PeerId:   pid.Pretty(),
		// auth provider empty
	}, nil
}

// NewKeypairBlock constructs a new Entity block
func NewKeypairBlock() block.Block {
	return &Entity{}
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
	if len(k.GetEntityId()) != 0 {
		if err := ValidateEntityID(k.GetEntityId()); err != nil {
			return err
		}
	}
	if err := ValidateDomainID(k.GetDomainId()); err != nil {
		return err
	}
	if err := ValidatePeerID(k.GetPeerId()); err != nil {
		return err
	}
	if k.GetAuthMethodId() == "" {
		if len(k.GetAuthMethodParams()) != 0 {
			return errors.New("auth provider params cannot be set unless auth provider id is set")
		}
	}
	return nil
}

// CheckMatchesEntity checks if the keypair matches the given entity.
func (k *Keypair) CheckMatchesEntity(e *Entity) error {
	if k.GetEntityId() != e.GetEntityId() {
		return errors.Errorf("entity id mismatch: %s != %s", k.GetEntityId(), e.GetEntityId())
	}
	if k.GetDomainId() != e.GetDomainId() {
		return errors.Errorf("domain id mismatch: %s != %s", k.GetDomainId(), e.GetDomainId())
	}
	return nil
}

// ParsePeerID parses the peer id field.
func (k *Keypair) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(k.GetPeerId())
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
