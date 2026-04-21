package identity

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
)

// NewEntity constructs a new entity object.
func NewEntity(domainID, entityID, entityUUID string) *Entity {
	return &Entity{
		DomainId:   domainID,
		EntityId:   entityID,
		EntityUuid: entityUUID,
		Epoch:      1,
	}
}

// EntityWithPrivKey builds a new Entity from a private key.
//
// authMethodID and authMethodParams can be empty.
func EntityWithPrivKey(
	domainID string,
	entityID, entityUUID string,
	privKey crypto.PrivKey,
	authMethodID string,
	authMethodParams []byte,
) (*Entity, error) {
	ent := NewEntity(domainID, entityID, entityUUID)
	pubKey := privKey.GetPublic()
	ekp, err := EntityKeypairWithPubKey(
		domainID, entityID,
		pubKey,
		authMethodID,
		authMethodParams,
	)
	if err != nil {
		return nil, err
	}
	err = ent.AppendKeypair(privKey, ekp)
	if err != nil {
		return nil, err
	}
	return ent, nil
}

// NewEntityBlock constructs a new Entity block
func NewEntityBlock() block.Block {
	return &Entity{}
}

// UnmarshalEntity unmarshals a Entity from a cursor.
// If empty, returns nil, nil
func UnmarshalEntity(ctx context.Context, bcs *block.Cursor) (*Entity, error) {
	return block.UnmarshalBlock[*Entity](ctx, bcs, NewEntityBlock)
}

// Validate validates the entity object and all keypair signatures.
// Auth method params and/or IDs are not validated.
func (e *Entity) Validate() error {
	if err := ValidateDomainID(e.GetDomainId()); err != nil {
		return err
	}
	if err := ValidateEntityID(e.GetEntityId()); err != nil {
		return err
	}
	if err := ValidateUUID(e.GetEntityUuid()); err != nil {
		return err
	}
	if _, err := e.UnmarshalVerifyKeypairs(); err != nil {
		return err
	}
	return nil
}

// AppendKeypair adds a keypair to the entity.
//
// Signs the keypair + entity data using the private key.
// The private key must match the given keypair.
// The keypair must not already exist.
func (e *Entity) AppendKeypair(privKey crypto.PrivKey, ekp *EntityKeypair) error {
	if e.EntityKeypairSet == nil {
		e.EntityKeypairSet = &EntityKeypairSet{}
	}
	return e.EntityKeypairSet.AppendKeypair(privKey, ekp, e)
}

// UnmarshalVerifyKeypairs unmarshals and checks the keypair signatures.
func (e *Entity) UnmarshalVerifyKeypairs() ([]*EntityKeypair, error) {
	return e.GetEntityKeypairSet().UnmarshalVerifyKeypairs(e)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Entity) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Entity) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*Entity)(nil))
