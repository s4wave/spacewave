package s4wave_sql_query_result

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
)

// SqlQueryResultTypeID is the world ObjectType id for SQL query results.
const SqlQueryResultTypeID = "sql/query-result"

// NewQueryResultBlock constructs a SQL query result block.
func NewQueryResultBlock() block.Block {
	return &QueryResult{}
}

// ReadQueryResultRoot reads a SQL query result object's root.
func ReadQueryResultRoot(ctx context.Context, ws world.WorldState, objectKey string) (*QueryResult, error) {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	return ReadQueryResultObjectRoot(ctx, obj)
}

// ReadQueryResultObjectRoot reads a SQL query result root from an object state.
func ReadQueryResultObjectRoot(ctx context.Context, obj world.ObjectState) (*QueryResult, error) {
	var result *QueryResult
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		result, err = block.UnmarshalBlock[*QueryResult](ctx, bcs, NewQueryResultBlock)
		if err != nil {
			return err
		}
		if result == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// WriteQueryResultRootRef writes a SQL query result root block and returns its ref.
func WriteQueryResultRootRef(
	ctx context.Context,
	ws world.WorldState,
	result *QueryResult,
) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(result, true)
		return nil
	})
}

// SyncResultGraphQuads replaces the result's source query and target database graph links.
func SyncResultGraphQuads(ctx context.Context, ws world.WorldState, objectKey string) error {
	result, err := ReadQueryResultRoot(ctx, ws, objectKey)
	if err != nil {
		return err
	}
	for _, pred := range []string{
		s4wave_sql.PredSqlQueryProducedBy.String(),
		s4wave_sql.PredSqlQueryAgainst.String(),
	} {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(objectKey, pred, "", ""), 0)
		if err != nil {
			return err
		}
		for _, q := range quads {
			if err := ws.DeleteGraphQuad(ctx, q); err != nil {
				return err
			}
		}
	}
	if result.GetSourceQueryObjectKey() != "" {
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
			objectKey,
			s4wave_sql.PredSqlQueryProducedBy.String(),
			result.GetSourceQueryObjectKey(),
			"",
		)); err != nil {
			return err
		}
	}
	if result.GetTargetDbObjectKey() == "" {
		return nil
	}
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		objectKey,
		s4wave_sql.PredSqlQueryAgainst.String(),
		result.GetTargetDbObjectKey(),
		"",
	))
}

// MarshalBlock marshals the SQL query result root.
func (r *QueryResult) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the SQL query result root.
func (r *QueryResult) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// Validate performs cursory checks on the SQL query result root.
func (r *QueryResult) Validate() error {
	return nil
}

// _ is a type assertion.
var _ block.Block = (*QueryResult)(nil)
