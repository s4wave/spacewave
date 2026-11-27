package identity_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	identity_domain "github.com/s4wave/spacewave/identity/domain"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

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
	return storeBlockUpdate(ctx, w, sender, key, di, func(ref *bucket.ObjectRef) world.Operation {
		return NewDomainInfoUpdateOp(ref)
	})
}

// Validate checks the operation cursorily.
// Should not block.
func (o *DomainInfoUpdateOp) Validate() error {
	if err := o.GetDomainInfoRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *DomainInfoUpdateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	kpRef := o.GetDomainInfoRef()

	resolve := func(ctx context.Context) (string, func() error, error) {
		di, err := FollowDomainInfo(ctx, worldHandle.AccessWorldState, kpRef)
		if err != nil {
			return "", nil, err
		}
		validate := func() error {
			if di.GetDomainId() == "" {
				return world.NewOpRefusedError(errors.New("domainInfo cannot be empty"))
			}
			return world.NewOpRefusedError(di.Validate())
		}
		return NewDomainInfoKey(di.GetDomainId()), validate, nil
	}

	if _, err := applyRefUpdate(ctx, worldHandle, kpRef, DomainInfoTypeID, resolve); err != nil {
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
	// Verify the referenced domain information before updating the object.
	domainInfoRef := o.GetDomainInfoRef()
	_, err = FollowDomainInfo(ctx, objectHandle.AccessWorldState, domainInfoRef)
	if err != nil {
		return false, err
	}

	// Replace the object's root reference with the validated domain information.
	_, err = objectHandle.SetRootRef(ctx, domainInfoRef)
	return false, err
}

// _ is a type assertion
var _ world.Operation = (*DomainInfoUpdateOp)(nil)
