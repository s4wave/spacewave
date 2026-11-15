package s4wave_sql_workbench_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	"github.com/sirupsen/logrus"
)

// NewSqlWorkbenchSetRootOp constructs a SQL workbench set-root operation.
func NewSqlWorkbenchSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlWorkbenchSetRootOp {
	return &SqlWorkbenchSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// NewSqlWorkbenchInitializeRootOp constructs a create-once SQL workbench root operation.
func NewSqlWorkbenchInitializeRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlWorkbenchSetRootOp {
	return &SqlWorkbenchSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone(), InitializeOnly: true}
}

// Validate performs cursory checks on the operation.
func (o *SqlWorkbenchSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/workbench: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the SQL workbench set-root operation to a world.
func (o *SqlWorkbenchSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Validate the root operation and require a SQL workbench target.
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_workbench.SqlWorkbenchTypeID); err != nil {
		return false, err
	}

	// Acquire the workbench object for the root update.
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}

	// Apply the root update before synchronizing the workbench graph.
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}

	return false, s4wave_sql_workbench.SyncWorkbenchGraphQuads(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL workbench set-root operation to an object.
func (o *SqlWorkbenchSetRootOp) ApplyWorldObjectOp(
	ctx context.Context,
	_ *logrus.Entry,
	os world.ObjectState,
	_ peer.ID,
) (bool, error) {
	// Validate the root operation against the supplied workbench object.
	if err := o.Validate(); err != nil {
		return false, err
	}
	if os == nil {
		return false, world.ErrObjectNotFound
	}
	if os.GetKey() != o.GetObjectKey() {
		return false, world.NewOpRefusedError(errors.Errorf("sql/workbench: op target %s does not match object %s", o.GetObjectKey(), os.GetKey()))
	}

	// Protect an existing workbench root from repeated initialization.
	if o.GetInitializeOnly() {
		rootRef, _, err := os.GetRootRef(ctx)
		if err != nil {
			return false, err
		}
		if rootRef != nil && !rootRef.GetRootRef().GetEmpty() {
			return false, world.NewOpRefusedError(ErrWorkbenchAlreadyInitialized)
		}
	}

	// Replace the workbench object root with the requested reference.
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

var _ block.Block = (*SqlWorkbenchSetRootOp)(nil)

var _ world.Operation = (*SqlWorkbenchSetRootOp)(nil)
