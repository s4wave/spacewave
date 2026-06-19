package s4wave_sql_query_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	"github.com/sirupsen/logrus"
)

// SqlQuerySetRootOpId is the world op id for advancing a sql/query root.
const SqlQuerySetRootOpId = "sql/query/set-root"

// NewSqlQuerySetRootOp constructs a SQL query set-root operation.
func NewSqlQuerySetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlQuerySetRootOp {
	return &SqlQuerySetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlQuerySetRootOp) GetOperationTypeId() string {
	return SqlQuerySetRootOpId
}

// Validate performs cursory checks on the operation.
func (o *SqlQuerySetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/query: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the SQL query set-root operation to a world.
func (o *SqlQuerySetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_query.SqlQueryTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}
	return false, s4wave_sql_query.SyncTargetDbQuad(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL query set-root operation to an object.
func (o *SqlQuerySetRootOp) ApplyWorldObjectOp(
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
		return false, errors.Errorf("sql/query: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

// MarshalBlock marshals the SQL query set-root operation.
func (o *SqlQuerySetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL query set-root operation.
func (o *SqlQuerySetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSqlQuerySetRootOp returns an empty SQL query set-root op for lookup.
func LookupSqlQuerySetRootOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == SqlQuerySetRootOpId {
		return &SqlQuerySetRootOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion.
var _ block.Block = (*SqlQuerySetRootOp)(nil)

// _ is a type assertion.
var _ world.Operation = (*SqlQuerySetRootOp)(nil)
