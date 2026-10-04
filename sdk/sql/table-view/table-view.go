package s4wave_sql_table_view

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
)

// SqlTableViewTypeID is the world ObjectType id for SQL table views.
const SqlTableViewTypeID = "sql/table-view"

// NewTableViewBlock constructs a SQL table view block.
func NewTableViewBlock() block.Block {
	return &TableView{}
}

// ReadTableViewRoot reads a SQL table view object's root.
func ReadTableViewRoot(ctx context.Context, ws world.WorldState, objectKey string) (*TableView, error) {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	return ReadTableViewObjectRoot(ctx, obj)
}

// ReadTableViewObjectRoot reads a SQL table view root from an object state.
func ReadTableViewObjectRoot(ctx context.Context, obj world.ObjectState) (*TableView, error) {
	var tableView *TableView
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		// Decode the table view root from the object block cursor.
		var err error
		tableView, err = block.UnmarshalBlock[*TableView](ctx, bcs, NewTableViewBlock)
		if err != nil {
			return err
		}

		// Require the object to contain a table view root.
		if tableView == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tableView, nil
}

// SyncTableViewGraphQuads replaces the table view's target schema graph link.
func SyncTableViewGraphQuads(ctx context.Context, ws world.WorldState, objectKey string) error {
	// Read the table view root that defines the target schema.
	tableView, err := ReadTableViewRoot(ctx, ws, objectKey)
	if err != nil {
		return err
	}

	// Load the current graph links from this view to schemas.
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(objectKey, s4wave_sql.PredSqlTableViewAgainstSchema.String(), "", ""),
		0,
	)
	if err != nil {
		return err
	}

	// Remove every previous target-schema link.
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}

	// Leave the graph unlinked when the view has no target schema.
	if tableView.GetTargetSchemaObjectKey() == "" {
		return nil
	}

	// Link the view to its configured target schema.
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		objectKey,
		s4wave_sql.PredSqlTableViewAgainstSchema.String(),
		tableView.GetTargetSchemaObjectKey(),
		"",
	))
}

// MarshalBlock marshals the SQL table view root.
func (v *TableView) MarshalBlock() ([]byte, error) {
	return v.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL table view root.
func (v *TableView) UnmarshalBlock(data []byte) error {
	return v.UnmarshalVT(data)
}

// Validate performs cursory checks on the SQL table view root.
func (v *TableView) Validate() error {
	return nil
}

var _ block.Block = (*TableView)(nil)
