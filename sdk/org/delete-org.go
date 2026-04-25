package s4wave_org

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// DeleteOrganizationOpID is the operation type ID.
var DeleteOrganizationOpID = "org/delete-organization"

// NewDeleteOrganizationOpBlock creates an empty block for deserialization.
func NewDeleteOrganizationOpBlock() block.Block {
	return &DeleteOrganizationOp{}
}

// GetOperationTypeId returns the operation type ID.
func (o *DeleteOrganizationOp) GetOperationTypeId() string {
	return DeleteOrganizationOpID
}

// Validate validates the operation.
func (o *DeleteOrganizationOp) Validate() error {
	if o.GetOrgObjectKey() == "" {
		return errors.New("org_object_key is required")
	}
	return nil
}

// MarshalBlock marshals the operation to bytes.
func (o *DeleteOrganizationOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the operation from bytes.
func (o *DeleteOrganizationOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyWorldOp applies the delete organization operation.
func (o *DeleteOrganizationOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	objKey := o.GetOrgObjectKey()

	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return true, err
	}
	if !found {
		return false, errors.New("organization object not found")
	}

	var state *OrgState
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uErr error
		state, uErr = UnmarshalOrgState(ctx, bcs)
		return uErr
	})
	if err != nil {
		return true, err
	}

	if state != nil && len(state.GetChildSharedObjects()) > 0 {
		return false, errors.New("cannot delete organization with child shared objects")
	}

	if _, err := ws.DeleteObject(ctx, objKey); err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp is not supported for this operation.
func (o *DeleteOrganizationOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupDeleteOrganizationOp looks up the delete organization operation.
func LookupDeleteOrganizationOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == DeleteOrganizationOpID {
		return &DeleteOrganizationOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*DeleteOrganizationOp)(nil)
