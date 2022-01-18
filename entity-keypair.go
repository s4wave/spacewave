package identity

import (
	"github.com/aperturerobotics/hydra/block"
	proto "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/pkg/errors"
)

// NewEntityKeypair constructs a new entity keypair binding.
func NewEntityKeypair(entityID, domainID string, kp *Keypair) *EntityKeypair {
	return &EntityKeypair{
		EntityId: entityID,
		DomainId: domainID,
		Keypair:  kp,
	}
}

// EntityKeypairWithPubKey builds a new EntityKeypair from a public key.
func EntityKeypairWithPubKey(entityID, domainID string, pubKey crypto.PubKey) (*EntityKeypair, error) {
	kp, err := NewKeypair(pubKey)
	if err != nil {
		return nil, err
	}
	return NewEntityKeypair(entityID, domainID, kp), nil
}

// NewEntityKeypairBlock constructs a new Entity block
func NewEntityKeypairBlock() block.Block {
	return &EntityKeypair{}
}

// UnmarshalEntityKeypair unmarshals a EntityKeypair from a cursor.
// If empty, returns nil, nil
func UnmarshalEntityKeypair(bcs *block.Cursor) (*EntityKeypair, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewEntityKeypairBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*EntityKeypair)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate validates the keypair.
func (k *EntityKeypair) Validate() error {
	if len(k.GetEntityId()) != 0 {
		if err := ValidateEntityID(k.GetEntityId()); err != nil {
			return err
		}
	}
	if err := ValidateDomainID(k.GetDomainId()); err != nil {
		return err
	}
	if err := k.GetKeypair().Validate(); err != nil {
		return errors.Wrap(err, "keypair")
	}
	return nil
}

// CheckMatchesEntity checks if the keypair matches the given entity.
func (k *EntityKeypair) CheckMatchesEntity(e *Entity) error {
	if k.GetEntityId() != e.GetEntityId() {
		return errors.Errorf("entity id mismatch: %s != %s", k.GetEntityId(), e.GetEntityId())
	}
	if k.GetDomainId() != e.GetDomainId() {
		return errors.Errorf("domain id mismatch: %s != %s", k.GetDomainId(), e.GetDomainId())
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (k *EntityKeypair) MarshalBlock() ([]byte, error) {
	return proto.Marshal(k)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (k *EntityKeypair) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, k)
}

// _ is a type assertion
var _ block.Block = ((*EntityKeypair)(nil))
