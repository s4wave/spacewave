// Package identity identifies a person or service across devices and binds keys
// to that identity.
//
// An Entity is an entity ID, akin to a username and a lowercase DNS label, in
// a domain, with a domain-unique UUID and an epoch that rises with each change.
// Its EntityKeypairSet lists the Keypairs that act for it. Each entry is
// signed by its own private key over the keypair and the entity's IDs, so a
// key proves its own membership and a set cannot list a key whose holder did
// not sign. Validate and UnmarshalVerifyKeypairs check every signature and
// that each entry records the entity's IDs. A Keypair records its peer ID, its
// PEM public key, and the auth method ID and parameters that derive its private
// key again, such as a password's scrypt parameters. The private key itself is
// never stored.
//
// The directives connect an entity to its keys at runtime. IdentityLookupEntity
// finds an Entity by domain and entity ID, SelectIdentityEntity and
// SelectEntityId choose the active one, DeriveEntityKeypair derives a private
// key for one of its Keypairs (the auth/derive controller resolves the
// directive), and PromptPassword asks the person for a secret when a method
// needs one. Package world stores entities, keypairs and domain info as World
// objects.
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
	// Construct the entity and its initial public keypair record.
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

	// Sign and append the initial keypair to the entity.
	err = ent.AppendKeypair(privKey, ekp)
	if err != nil {
		return nil, err
	}

	return ent, nil
}

// UnmarshalEntity unmarshals an Entity from a cursor.
// If the cursor is empty, it returns nil, nil.
func UnmarshalEntity(ctx context.Context, bcs *block.Cursor) (*Entity, error) {
	return block.UnmarshalBlock[Entity](ctx, bcs)
}

// Validate validates the entity object and all keypair signatures.
// Auth method params and/or IDs are not validated.
func (e *Entity) Validate() error {
	// Validate the entity domain, identifier, and UUID.
	if err := ValidateDomainID(e.GetDomainId()); err != nil {
		return err
	}
	if err := ValidateEntityID(e.GetEntityId()); err != nil {
		return err
	}
	if err := ValidateUUID(e.GetEntityUuid()); err != nil {
		return err
	}

	// Verify the entity keypair records and their signatures.
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

// _ is a type assertion
var _ block.Block = (*Entity)(nil)
