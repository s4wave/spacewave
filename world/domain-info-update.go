package identity_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DomainInfoUpdateOpId is the domain info update operation id.
var DomainInfoUpdateOpId = DomainInfoTypeID + "/update"

// NewDomainInfoUpdateOp constructs a new DomainInfoUpdateOp block.
func NewDomainInfoUpdateOp(domainInfoRef *bucket.ObjectRef) *DomainInfoUpdateOp {
	return &DomainInfoUpdateOp{
		DomainInfoRef: domainInfoRef,
	}
}

// StoreDomainInfo stores a domain info to a object using DomainInfoUpdate.
// Returns seqno, sysErr, error.
func StoreDomainInfo(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	di *identity_domain.DomainInfo,
) (uint64, bool, error) {
	domainID := di.GetDomainId()
	if err := identity.ValidateDomainID(domainID); err != nil {
		return 0, false, err
	}

	key := NewDomainInfoKey(domainID)
	obj, objFound, err := w.GetObject(key)
	if err != nil {
		return 0, false, err
	}
	setDomainInfo := func(bcs *block.Cursor) error {
		bcs.SetBlock(di, true)
		bcs.ClearAllRefs()
		return nil
	}
	var kpRef *bucket.ObjectRef
	if objFound {
		var changed bool
		kpRef, changed, err = world.AccessObjectState(ctx, obj, false, setDomainInfo)
		if err != nil || !changed {
			return 0, false, err
		}
	} else {
		kpRef, err = world.AccessObject(ctx, w.AccessWorldState, nil, setDomainInfo)
		if err != nil {
			return 0, false, err
		}
	}

	op := NewDomainInfoUpdateOp(kpRef)
	return w.ApplyWorldOp(op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *DomainInfoUpdateOp) Validate() error {
	if err := o.GetDomainInfoRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *DomainInfoUpdateOp) GetOperationTypeId() string {
	return DomainInfoUpdateOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *DomainInfoUpdateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	kpRef := o.GetDomainInfoRef()

	// create / validate the objectref
	var di *identity_domain.DomainInfo
	di, err = FollowDomainInfo(ctx, worldHandle.AccessWorldState, kpRef)
	if err == nil && di.GetDomainId() == "" {
		err = errors.New("domainInfo cannot be empty")
	}
	if err != nil {
		return false, err
	}

	if err := di.Validate(); err != nil {
		return false, err
	}

	domainID := di.GetDomainId()
	objKey := NewDomainInfoKey(domainID)

	// create the object if it doesn't exist.
	obj, objFound, err := worldHandle.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if objFound {
		_, err = obj.SetRootRef(kpRef)
		return false, err
	}

	_, err = worldHandle.CreateObject(objKey, kpRef)
	if err != nil {
		return false, err
	}

	// DomainInfo type -> types/identity/domain
	typesState := world_types.NewTypesState(ctx, worldHandle)
	if err := typesState.SetObjectType(objKey, DomainInfoTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *DomainInfoUpdateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Applying to an existing object.
	domainInfoRef := o.GetDomainInfoRef()
	_, err = FollowDomainInfo(ctx, objectHandle.AccessWorldState, domainInfoRef)
	if err != nil {
		return false, err
	}

	// update the object
	_, err = objectHandle.SetRootRef(domainInfoRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *DomainInfoUpdateOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *DomainInfoUpdateOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*DomainInfoUpdateOp)(nil))
