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

// SqlWorkbenchSetRootOpId is the world op id for advancing a sql/workbench root.
const SqlWorkbenchSetRootOpId = "sql/workbench/set-root"

// NewSqlWorkbenchSetRootOp constructs a SQL workbench set-root operation.
func NewSqlWorkbenchSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlWorkbenchSetRootOp {
	return &SqlWorkbenchSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// NewSqlWorkbenchInitializeRootOp constructs a create-once SQL workbench root operation.
func NewSqlWorkbenchInitializeRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlWorkbenchSetRootOp {
	return &SqlWorkbenchSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone(), InitializeOnly: true}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlWorkbenchSetRootOp) GetOperationTypeId() string {
	return SqlWorkbenchSetRootOpId
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
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_workbench.SqlWorkbenchTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
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
	if err := o.Validate(); err != nil {
		return false, err
	}
	if os == nil {
		return false, world.ErrObjectNotFound
	}
	if os.GetKey() != o.GetObjectKey() {
		return false, errors.Errorf("sql/workbench: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	if o.GetInitializeOnly() {
		rootRef, _, err := os.GetRootRef(ctx)
		if err != nil {
			return false, err
		}
		if rootRef != nil && !rootRef.GetRootRef().GetEmpty() {
			return false, ErrWorkbenchAlreadyInitialized
		}
	}
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

// MarshalBlock marshals the SQL workbench set-root operation.
func (o *SqlWorkbenchSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL workbench set-root operation.
func (o *SqlWorkbenchSetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSqlWorkbenchSetRootOp returns an empty SQL workbench set-root op for lookup.
func LookupSqlWorkbenchSetRootOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == SqlWorkbenchSetRootOpId {
		return &SqlWorkbenchSetRootOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion.
var _ block.Block = (*SqlWorkbenchSetRootOp)(nil)

// _ is a type assertion.
var _ world.Operation = (*SqlWorkbenchSetRootOp)(nil)
