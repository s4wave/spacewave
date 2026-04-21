package identity_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	identity_domain "github.com/s4wave/spacewave/identity/domain"
	"github.com/s4wave/spacewave/net/peer"
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
	obj, objFound, err := w.GetObject(ctx, key)
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
	return w.ApplyWorldOp(ctx, op, sender)
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
	obj, objFound, err := worldHandle.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if objFound {
		_, err = obj.SetRootRef(ctx, kpRef)
		return false, err
	}

	_, err = worldHandle.CreateObject(ctx, objKey, kpRef)
	if err != nil {
		return false, err
	}

	// DomainInfo type -> types/identity/domain
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, DomainInfoTypeID); err != nil {
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
	_, err = objectHandle.SetRootRef(ctx, domainInfoRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *DomainInfoUpdateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *DomainInfoUpdateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*DomainInfoUpdateOp)(nil))
