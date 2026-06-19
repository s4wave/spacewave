package s4wave_sql_schema

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
)

// SqlSchemaTypeID is the world ObjectType id for SQL schemas.
const SqlSchemaTypeID = "sql/schema"

// NewSchemaBlock constructs a SQL schema block.
func NewSchemaBlock() block.Block {
	return &Schema{}
}

// ReadSchemaRoot reads a SQL schema object's root.
func ReadSchemaRoot(ctx context.Context, ws world.WorldState, objectKey string) (*Schema, error) {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	return ReadSchemaObjectRoot(ctx, obj)
}

// ReadSchemaObjectRoot reads a SQL schema root from an object state.
func ReadSchemaObjectRoot(ctx context.Context, obj world.ObjectState) (*Schema, error) {
	var schema *Schema
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		schema, err = block.UnmarshalBlock[*Schema](ctx, bcs, NewSchemaBlock)
		if err != nil {
			return err
		}
		if schema == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return schema, nil
}

// WriteSchemaRootRef writes a SQL schema root block and returns its ref.
func WriteSchemaRootRef(ctx context.Context, ws world.WorldState, schema *Schema) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(schema, true)
		return nil
	})
}

// SyncSchemaGraphQuads replaces the schema's target database graph link.
func SyncSchemaGraphQuads(ctx context.Context, ws world.WorldState, objectKey string) error {
	schema, err := ReadSchemaRoot(ctx, ws, objectKey)
	if err != nil {
		return err
	}
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(objectKey, s4wave_sql.PredSqlSchemaInDb.String(), "", ""),
		0,
	)
	if err != nil {
		return err
	}
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}
	if schema.GetTargetDbObjectKey() == "" {
		return nil
	}
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		objectKey,
		s4wave_sql.PredSqlSchemaInDb.String(),
		schema.GetTargetDbObjectKey(),
		"",
	))
}

// MarshalBlock marshals the SQL schema root.
func (s *Schema) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL schema root.
func (s *Schema) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate performs cursory checks on the SQL schema root.
func (s *Schema) Validate() error {
	return nil
}

// _ is a type assertion.
var _ block.Block = (*Schema)(nil)
