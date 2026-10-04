package s4wave_sql_query

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
)

// SqlQueryTypeID is the world ObjectType id for SQL queries.
const SqlQueryTypeID = "sql/query"

// SqlQueryBlockTypeID is the block type id for SQL query roots.
const SqlQueryBlockTypeID = "github.com/s4wave/spacewave/sdk/sql/query.Query"

// SqlQueryBlockType constructs SQL query root blocks for typed cursor writes.
var SqlQueryBlockType = blocktype.NewBlockType(
	SqlQueryBlockTypeID,
	func() *Query { return &Query{} },
)

// NewQueryBlock constructs a SQL query block.
func NewQueryBlock() block.Block {
	return &Query{}
}

// ReadQueryRoot reads a SQL query object's root.
func ReadQueryRoot(ctx context.Context, ws world.WorldState, objectKey string) (*Query, error) {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	return ReadQueryObjectRoot(ctx, obj)
}

// ReadQueryObjectRoot reads a SQL query root from an object state.
func ReadQueryObjectRoot(ctx context.Context, obj world.ObjectState) (*Query, error) {
	var query *Query
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		// Decode the query root from the object block cursor.
		var err error
		query, err = block.UnmarshalBlock[*Query](ctx, bcs, NewQueryBlock)
		if err != nil {
			return err
		}

		// Require the object to contain a query root.
		if query == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return query, nil
}

// WriteQueryRootRef writes a SQL query root block and returns its ref.
// Write through a stage held until the root is adopted.
func WriteQueryRootRef(ctx context.Context, storage world.WorldStorage, query *Query) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, storage.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(query, true)
		return nil
	})
}

// SyncTargetDbQuad replaces the query's target database graph link.
func SyncTargetDbQuad(ctx context.Context, ws world.WorldState, objectKey string) error {
	// Read the query root that defines the target database.
	query, err := ReadQueryRoot(ctx, ws, objectKey)
	if err != nil {
		return err
	}

	// Load the current target-database graph links.
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(objectKey, s4wave_sql.PredSqlQueryAgainst.String(), "", ""),
		0,
	)
	if err != nil {
		return err
	}

	// Remove every previous target-database link.
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}

	// Leave the query unlinked when it has no target database.
	if query.GetTargetDbObjectKey() == "" {
		return nil
	}

	// Link the query to its configured target database.
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		objectKey,
		s4wave_sql.PredSqlQueryAgainst.String(),
		query.GetTargetDbObjectKey(),
		"",
	))
}

// MarshalBlock marshals the SQL query root.
func (q *Query) MarshalBlock() ([]byte, error) {
	return q.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL query root.
func (q *Query) UnmarshalBlock(data []byte) error {
	return q.UnmarshalVT(data)
}

// Validate performs cursory checks on the SQL query root.
func (q *Query) Validate() error {
	return nil
}

var _ block.Block = (*Query)(nil)
