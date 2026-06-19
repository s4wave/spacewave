package s4wave_sql_table_view_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	"github.com/sirupsen/logrus"
)

// SqlTableViewSetRootOpId is the world op id for advancing a sql/table-view root.
const SqlTableViewSetRootOpId = "sql/table-view/set-root"

// NewSqlTableViewSetRootOp constructs a SQL table view set-root operation.
func NewSqlTableViewSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *SqlTableViewSetRootOp {
	return &SqlTableViewSetRootOp{ObjectKey: objectKey, RootRef: rootRef.Clone()}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlTableViewSetRootOp) GetOperationTypeId() string {
	return SqlTableViewSetRootOpId
}

// Validate performs cursory checks on the operation.
func (o *SqlTableViewSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/table-view: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the SQL table view set-root operation to a world.
func (o *SqlTableViewSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), s4wave_sql_table_view.SqlTableViewTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	if sysErr, err := o.ApplyWorldObjectOp(ctx, le, obj, sender); err != nil || sysErr {
		return sysErr, err
	}
	return false, s4wave_sql_table_view.SyncTableViewGraphQuads(ctx, ws, o.GetObjectKey())
}

// ApplyWorldObjectOp applies the SQL table view set-root operation to an object.
func (o *SqlTableViewSetRootOp) ApplyWorldObjectOp(
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
		return false, errors.Errorf("sql/table-view: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	_, err := os.SetRootRef(ctx, o.GetRootRef())
	return false, err
}

// MarshalBlock marshals the SQL table view set-root operation.
func (o *SqlTableViewSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL table view set-root operation.
func (o *SqlTableViewSetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSqlTableViewSetRootOp returns an empty SQL table view set-root op for lookup.
func LookupSqlTableViewSetRootOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == SqlTableViewSetRootOpId {
		return &SqlTableViewSetRootOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion.
var _ block.Block = (*SqlTableViewSetRootOp)(nil)

// _ is a type assertion.
var _ world.Operation = (*SqlTableViewSetRootOp)(nil)
