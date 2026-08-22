package s4wave_sql_query_result_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	"github.com/sirupsen/logrus"
)

// SqlQueryResultSetRootOpId is the world op id for advancing a sql/query-result root.
const SqlQueryResultSetRootOpId = "sql/query-result/set-root"

// NewSqlQueryResultSetRootOp constructs a SQL query result set-root operation.
func NewSqlQueryResultSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlQueryResultSetRootOp {
	return &SqlQueryResultSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlQueryResultSetRootOp) GetOperationTypeId() string {
	return SqlQueryResultSetRootOpId
}

// Validate performs cursory checks on the operation.
func (o *SqlQueryResultSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/query-result: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the SQL query result set-root operation to a world.
func (o *SqlQueryResultSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}
	return false, s4wave_sql_query_result.SyncResultGraphQuads(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL query result set-root operation to an object.
func (o *SqlQueryResultSetRootOp) ApplyWorldObjectOp(
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
		return false, errors.Errorf("sql/query-result: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

// MarshalBlock marshals the SQL query result set-root operation.
func (o *SqlQueryResultSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL query result set-root operation.
func (o *SqlQueryResultSetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSqlQueryResultSetRootOp returns an empty SQL query result set-root op for lookup.
func LookupSqlQueryResultSetRootOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == SqlQueryResultSetRootOpId {
		return &SqlQueryResultSetRootOp{}, nil
	}
	return nil, nil
}

var _ block.Block = (*SqlQueryResultSetRootOp)(nil)

var _ world.Operation = (*SqlQueryResultSetRootOp)(nil)
