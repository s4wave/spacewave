package s4wave_sql_schema_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	"github.com/sirupsen/logrus"
)

// NewSqlSchemaSetRootOp constructs a SQL schema set-root operation.
func NewSqlSchemaSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlSchemaSetRootOp {
	return &SqlSchemaSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// Validate performs cursory checks on the operation.
func (o *SqlSchemaSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/schema: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the SQL schema set-root operation to a world.
func (o *SqlSchemaSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Validate the SQL schema root operation and its target object type.
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		return false, err
	}

	// Acquire the SQL schema object and apply its new root reference.
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}

	// Synchronize the schema graph after publishing the new root reference.
	return false, s4wave_sql_schema.SyncSchemaGraphQuads(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL schema set-root operation to an object.
func (o *SqlSchemaSetRootOp) ApplyWorldObjectOp(
	ctx context.Context,
	_ *logrus.Entry,
	os world.ObjectState,
	_ peer.ID,
) (bool, error) {
	// Validate the root operation and require its matching SQL schema object.
	if err := o.Validate(); err != nil {
		return false, err
	}
	if os == nil {
		return false, world.ErrObjectNotFound
	}
	if os.GetKey() != o.GetObjectKey() {
		return false, world.NewOpRefusedError(errors.Errorf("sql/schema: op target %s does not match object %s", o.GetObjectKey(), os.GetKey()))
	}

	// Publish the root reference on the matching SQL schema object.
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

var _ block.Block = (*SqlSchemaSetRootOp)(nil)

var _ world.Operation = (*SqlSchemaSetRootOp)(nil)
