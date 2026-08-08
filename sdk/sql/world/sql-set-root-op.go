package s4wave_sql_world

import (
	"context"
	"database/sql/driver"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SqlSetRootOpId is the world operation id for sql/db root updates.
var SqlSetRootOpId = "sql/db/set-root"

const sqlSetRootMaxCASAttempts = 16

// NewSqlSetRootOp constructs a sql/db root update operation.
func NewSqlSetRootOp(
	objectKey string,
	baseRef *bucket.ObjectRef,
	rootRef *bucket.ObjectRef,
	statements []*SqlStatement,
) *SqlSetRootOp {
	return &SqlSetRootOp{
		ObjectKey:  objectKey,
		BaseRef:    baseRef.Clone(),
		RootRef:    rootRef.Clone(),
		Statements: cloneSqlStatements(statements),
	}
}

// NewSqlSetRootOpBlock constructs a SqlSetRootOp block.
func NewSqlSetRootOpBlock() block.Block {
	return &SqlSetRootOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *SqlSetRootOp) GetOperationTypeId() string {
	return SqlSetRootOpId
}

// Validate performs cursory checks on the operation.
func (o *SqlSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("sql/db: root ref is required")
	}
	if err := rootRef.Validate(); err != nil {
		return err
	}
	// An empty base ref is the object's initial root: a first commit from the
	// empty root carries no base. Validate the base only when it is populated.
	if baseRef := o.GetBaseRef(); baseRef != nil && !baseRef.GetEmpty() {
		if err := baseRef.Validate(); err != nil {
			return err
		}
	}
	for _, statement := range o.GetStatements() {
		if statement == nil {
			return errors.New("sql/db: statement is required")
		}
		if statement.GetQuery() == "" {
			return errors.New("sql/db: statement query is required")
		}
		switch statement.GetKind() {
		case SqlStatementKind_SQL_STATEMENT_KIND_EXEC:
		case SqlStatementKind_SQL_STATEMENT_KIND_QUERY:
		default:
			return errors.New("sql/db: invalid statement kind")
		}
		for _, arg := range statement.GetArgs() {
			if arg == nil {
				return errors.New("sql/db: statement argument is required")
			}
		}
	}
	return nil
}

// ApplyWorldOp applies the root update to a sql/db world object.
func (o *SqlSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), SqlDbTypeID); err != nil {
		return false, err
	}
	os, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	return o.ApplyWorldObjectOp(ctx, le, os, sender)
}

// ApplyWorldObjectOp applies the root update to an already-resolved object.
func (o *SqlSetRootOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if os.GetKey() != o.GetObjectKey() {
		return false, errors.Errorf("sql/db: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	// Divergent commits replay this transaction's statements on the current
	// root; conflicts resolve in world-op order by SQL statement semantics.
	expected := o.GetBaseRef().Clone()
	nextRoot := o.GetRootRef().Clone()
	for range sqlSetRootMaxCASAttempts {
		current, _, err := os.GetRootRef(ctx)
		if err != nil {
			return false, err
		}
		if current.EqualsRef(expected) {
			_, err := os.SetRootRef(ctx, nextRoot.Clone())
			return false, err
		}
		nextRoot, err = o.rebaseRoot(ctx, os, current)
		if err != nil {
			return false, err
		}
		expected = current.Clone()
	}
	return false, errors.New("sql/db: root advance CAS attempts exhausted")
}

func (o *SqlSetRootOp) rebaseRoot(
	ctx context.Context,
	os world.ObjectState,
	currentRoot *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	var nextRoot *bucket.ObjectRef
	err := os.AccessWorldState(ctx, currentRoot, func(root *bucket_lookup.Cursor) error {
		rootCursor := root.Clone()
		defer rootCursor.Release()
		db := sql_mysql.NewMysql(rootCursor, func(root *bucket.ObjectRef) error {
			nextRoot = root.Clone()
			return nil
		})
		dsn := ""
		if statements := o.GetStatements(); len(statements) != 0 {
			dsn = statements[0].GetDsn()
		}
		tx, err := db.NewSqlTransaction(ctx, true, dsn)
		if err != nil {
			return err
		}
		defer tx.Discard()
		ops, err := tx.GetSqlOps(ctx)
		if err != nil {
			return err
		}
		for _, statement := range o.GetStatements() {
			if statement.GetDsn() != dsn {
				return errors.New("sql/db: mixed transaction DSNs cannot be rebased")
			}
			args := sqlStatementNamedValues(statement)
			switch statement.GetKind() {
			case SqlStatementKind_SQL_STATEMENT_KIND_EXEC:
				if _, err := ops.ExecContext(ctx, statement.GetQuery(), args); err != nil {
					return err
				}
			case SqlStatementKind_SQL_STATEMENT_KIND_QUERY:
				rows, err := ops.QueryContext(ctx, statement.GetQuery(), args)
				if err != nil {
					return err
				}
				if err := drainSqlRows(rows); err != nil {
					return err
				}
			default:
				return errors.New("sql/db: invalid statement kind")
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if nextRoot == nil {
			nextRoot = db.GetRootNodeRef()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if nextRoot == nil || nextRoot.GetEmpty() {
		return nil, errors.New("sql/db: rebased root was not captured")
	}
	return nextRoot, nil
}

func buildSqlStatement(
	kind SqlStatementKind,
	dsn string,
	query string,
	args []driver.NamedValue,
) (*SqlStatement, error) {
	out := &SqlStatement{
		Kind:  kind,
		Dsn:   dsn,
		Query: query,
	}
	for _, arg := range args {
		value, err := sql_rpc.DriverValueToSqlValue(arg.Value)
		if err != nil {
			return nil, err
		}
		out.Args = append(out.Args, &SqlArgument{
			Name:    arg.Name,
			Ordinal: int32(arg.Ordinal),
			Value:   value,
		})
	}
	return out, nil
}

func sqlStatementNamedValues(statement *SqlStatement) []driver.NamedValue {
	args := statement.GetArgs()
	if len(args) == 0 {
		return nil
	}
	out := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		out[i] = driver.NamedValue{
			Name:    arg.GetName(),
			Ordinal: int(arg.GetOrdinal()),
			Value:   sql_rpc.SqlValueToDriverValue(arg.GetValue()),
		}
		if out[i].Ordinal == 0 {
			out[i].Ordinal = i + 1
		}
	}
	return out
}

func drainSqlRows(rows driver.Rows) error {
	defer rows.Close()
	values := make([]driver.Value, len(rows.Columns()))
	for {
		err := rows.Next(values)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func cloneSqlStatements(statements []*SqlStatement) []*SqlStatement {
	if len(statements) == 0 {
		return nil
	}
	out := make([]*SqlStatement, len(statements))
	for i, statement := range statements {
		out[i] = statement.CloneVT()
	}
	return out
}

// MarshalBlock marshals the operation block.
func (o *SqlSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the operation block.
func (o *SqlSetRootOp) UnmarshalBlock(b []byte) error {
	return o.UnmarshalVT(b)
}

// LookupSqlSetRootOp constructs a sql/db root update operation from its id.
func LookupSqlSetRootOp(ctx context.Context, id string) (world.Operation, error) {
	if id == SqlSetRootOpId {
		return &SqlSetRootOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*SqlSetRootOp)(nil)
