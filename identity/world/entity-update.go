package identity_world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// EntityUpdateOpId is the entity update operation id.
var EntityUpdateOpId = EntityTypeID + "/update"

// NewEntityUpdateOp constructs a new EntityUpdateOp block.
func NewEntityUpdateOp(entityRef *bucket.ObjectRef) *EntityUpdateOp {
	return &EntityUpdateOp{
		EntityRef: entityRef,
	}
}

// StoreEntity stores an entity to an object using EntityUpdate.
// Returns seqno, sysErr, error.
func StoreEntity(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	entity *identity.Entity,
) (uint64, bool, error) {
	domainID, entityID := entity.GetDomainId(), entity.GetEntityId()
	key := NewEntityKey(domainID, entityID)
	return storeBlockUpdate(ctx, w, sender, key, entity, func(ref *bucket.ObjectRef) world.Operation {
		return NewEntityUpdateOp(ref)
	})
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *EntityUpdateOp) Validate() error {
	if err := o.GetEntityRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *EntityUpdateOp) GetOperationTypeId() string {
	return EntityUpdateOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *EntityUpdateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	entityRef := o.GetEntityRef()

	resolve := func(ctx context.Context) (string, func() error, error) {
		entity, err := FollowEntity(ctx, worldHandle.AccessWorldState, entityRef)
		if err != nil || entity == nil {
			return "", nil, err
		}
		validate := func() error { return nil }
		domainID, entityID := entity.GetDomainId(), entity.GetEntityId()
		return NewEntityKey(domainID, entityID), validate, nil
	}

	if _, err := applyRefUpdate(ctx, worldHandle, entityRef, EntityTypeID, resolve); err != nil {
		return false, err
	}

	// Re-resolve the entity to link its keypairs and domain info.
	entity, err := FollowEntity(ctx, worldHandle.AccessWorldState, entityRef)
	if err != nil {
		return false, err
	}

	// Persist newly referenced keypairs and collect their object keys.
	entityKps, err := entity.UnmarshalVerifyKeypairs()
	if err != nil {
		return false, err
	}
	kps := make([]*identity.Keypair, len(entityKps))
	for i, ekp := range entityKps {
		kps[i] = ekp.GetKeypair()
	}
	kpObjectKeys, err := EnsureKeypairsExist(ctx, worldHandle, sender, kps, false)
	if err != nil {
		return false, err
	}

	domainID, entityID := entity.GetDomainId(), entity.GetEntityId()
	objKey := NewEntityKey(domainID, entityID)

	// Link the entity to each referenced keypair.
	for _, kpObjKey := range kpObjectKeys {
		kpQuad := NewObjectToKeypairQuad(objKey, kpObjKey)
		if err := worldHandle.SetGraphQuad(ctx, kpQuad); err != nil {
			return false, err
		}
	}

	// Link the entity to its domain information when available.
	diKey := NewDomainInfoKey(domainID)
	_, diExists, err := worldHandle.GetObject(ctx, diKey)
	if err != nil {
		return false, err
	}
	if diExists {
		diQuad := NewEntityToDomainInfoQuad(objKey, diKey)
		if err := worldHandle.SetGraphQuad(ctx, diQuad); err != nil {
			return false, err
		}
	}
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *EntityUpdateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *EntityUpdateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *EntityUpdateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = (*EntityUpdateOp)(nil)
