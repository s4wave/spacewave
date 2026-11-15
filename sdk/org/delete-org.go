package s4wave_org

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Validate validates the operation.
func (o *DeleteOrganizationOp) Validate() error {
	if o.GetOrgObjectKey() == "" {
		return errors.New("org_object_key is required")
	}
	return nil
}

// ApplyWorldOp applies the delete organization operation.
func (o *DeleteOrganizationOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Open the requested organization object for deletion and release its handle afterward.
	objKey := o.GetOrgObjectKey()
	objState, found, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return true, err
	}
	if !found {
		return false, world.NewOpRefusedError(errors.New("organization object not found"))
	}

	// Read the organization body to check its child shared objects.
	var state *OrgState
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uErr error
		state, uErr = UnmarshalOrgState(ctx, bcs)
		return uErr
	})
	if err != nil {
		return true, err
	}

	// Keep organizations whose child shared objects still depend on them.
	if state != nil && len(state.GetChildSharedObjects()) > 0 {
		return false, world.NewOpRefusedError(errors.New("cannot delete organization with child shared objects"))
	}

	// Remove the organization object from the World.
	if _, err := ws.DeleteObject(ctx, objKey); err != nil {
		return true, err
	}

	return false, nil
}

var _ world.Operation = (*DeleteOrganizationOp)(nil)
