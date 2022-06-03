package identity_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	"google.golang.org/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// NOTE: This code is nearly identical to session-update.go
// perhaps it could be code-genned or replaced w/ a common struct.

// EntityUpdateOpId is the entity update operation id.
var EntityUpdateOpId = EntityTypeID + "/update"

// NewEntityUpdateOp constructs a new EntityUpdateOp block.
func NewEntityUpdateOp(entityRef *bucket.ObjectRef) *EntityUpdateOp {
	return &EntityUpdateOp{
		EntityRef: entityRef,
	}
}

// StoreEntity stores a session to a object using EntityUpdate.
// Returns seqno, sysErr, error.
func StoreEntity(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	entity *identity.Entity,
) (uint64, bool, error) {
	domainID, entityID := entity.GetDomainId(), entity.GetEntityId()
	key := NewEntityKey(domainID, entityID)
	obj, objFound, err := w.GetObject(key)
	if err != nil {
		return 0, false, err
	}
	setEntity := func(bcs *block.Cursor) error {
		bcs.SetBlock(entity, true)
		bcs.ClearAllRefs()
		return nil
	}
	var sessRef *bucket.ObjectRef
	if objFound {
		var changed bool
		sessRef, changed, err = world.AccessObjectState(ctx, obj, false, setEntity)
		if err != nil || !changed {
			return 0, false, err
		}
	} else {
		sessRef, err = world.AccessObject(ctx, w.AccessWorldState, nil, setEntity)
		if err != nil {
			return 0, false, err
		}
	}

	op := NewEntityUpdateOp(sessRef)
	return w.ApplyWorldOp(op, sender)
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

	// create / validate the objectref
	var entity *identity.Entity
	entity, err = FollowEntity(ctx, worldHandle.AccessWorldState, entityRef)
	if err != nil || entity == nil {
		return false, err
	}

	domainID, entityID := entity.GetDomainId(), entity.GetEntityId()
	objKey := NewEntityKey(domainID, entityID)

	// create the entity if it doesn't exist.
	obj, objFound, err := worldHandle.GetObject(objKey)
	if err != nil {
		return false, err
	}

	prevLinkedKp := make(map[string]struct{})
	if objFound {
		// Build list of previous linked keypairs.
		kpObjectIDs, err := ListEntityKeypairs(ctx, worldHandle, objKey)
		if err != nil {
			return false, err
		}
		for _, id := range kpObjectIDs {
			prevLinkedKp[id] = struct{}{}
		}

		// TODO: Verify at least one of the old keypairs signed-off.
		_, err = obj.SetRootRef(entityRef)
		if err != nil {
			return false, err
		}
	} else {
		_, err = worldHandle.CreateObject(objKey, entityRef)
		if err != nil {
			return false, err
		}

		// Entity type
		typesState := world_types.NewTypesState(ctx, worldHandle)
		if err := typesState.SetObjectType(objKey, EntityTypeID); err != nil {
			return false, err
		}
	}

	// Add new keypairs to storage if they don't exist.
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

	// Create/update links to keypairs.
	for _, kpObjKey := range kpObjectKeys {
		kpQuad := NewObjectToKeypairQuad(objKey, kpObjKey)
		if err := worldHandle.SetGraphQuad(kpQuad); err != nil {
			return false, err
		}
	}

	// Create/update link to domain info (if exists)
	diKey := NewDomainInfoKey(domainID)
	_, diExists, err := worldHandle.GetObject(diKey)
	if err != nil {
		return false, err
	}
	if diExists {
		diQuad := NewEntityToDomainInfoQuad(objKey, diKey)
		if err := worldHandle.SetGraphQuad(diQuad); err != nil {
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
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *EntityUpdateOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*EntityUpdateOp)(nil))
