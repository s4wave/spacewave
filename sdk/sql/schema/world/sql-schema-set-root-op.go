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

// SqlSchemaSetRootOpId is the world op id for advancing a sql/schema root.
const SqlSchemaSetRootOpId = "sql/schema/set-root"

// NewSqlSchemaSetRootOp constructs a SQL schema set-root operation.
func NewSqlSchemaSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlSchemaSetRootOp {
	return &SqlSchemaSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlSchemaSetRootOp) GetOperationTypeId() string {
	return SqlSchemaSetRootOpId
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
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}
	return false, s4wave_sql_schema.SyncSchemaGraphQuads(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL schema set-root operation to an object.
func (o *SqlSchemaSetRootOp) ApplyWorldObjectOp(
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
		return false, errors.Errorf("sql/schema: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

// MarshalBlock marshals the SQL schema set-root operation.
func (o *SqlSchemaSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL schema set-root operation.
func (o *SqlSchemaSetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSqlSchemaSetRootOp returns an empty SQL schema set-root op for lookup.
func LookupSqlSchemaSetRootOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == SqlSchemaSetRootOpId {
		return &SqlSchemaSetRootOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion.
var _ block.Block = (*SqlSchemaSetRootOp)(nil)

// _ is a type assertion.
var _ world.Operation = (*SqlSchemaSetRootOp)(nil)
