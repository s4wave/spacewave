//go:build !tinygo

package s4wave_sql_query_result

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// SqlQueryResultResource serves SqlQueryResultResourceService for one result object.
type SqlQueryResultResource struct {
	ws        world.WorldState
	objectKey string
	mux       srpc.Mux
}

// NewSqlQueryResultResource constructs a SQL query result resource.
func NewSqlQueryResultResource(
	ws world.WorldState,
	objectKey string,
) *SqlQueryResultResource {
	r := &SqlQueryResultResource{
		ws:        ws,
		objectKey: objectKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterSqlQueryResultResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *SqlQueryResultResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the resource lifecycle.
func (r *SqlQueryResultResource) Close() {}

// GetResultGrid returns the persisted result grid.
func (r *SqlQueryResultResource) GetResultGrid(
	ctx context.Context,
	_ *GetResultGridRequest,
) (*GetResultGridResponse, error) {
	// Require a World state before accessing the SQL query result.
	if r.ws == nil {
		return nil, errors.New("sql/query-result: world state is required")
	}

	// Verify that the resource object has the SQL query result type.
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, SqlQueryResultTypeID); err != nil {
		return nil, errors.Wrap(err, "sql/query-result: check object type")
	}

	// Read the persisted SQL query result root.
	result, err := ReadQueryResultRoot(ctx, r.ws, r.objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "sql/query-result: read result root")
	}

	// Clone the SQL query result fields for the grid response.
	cloned := result.CloneVT()
	return &GetResultGridResponse{
		Columns:              cloned.GetColumns(),
		RowBatches:           cloned.GetRowBatches(),
		ExecutedAt:           cloned.GetExecutedAt(),
		Truncated:            cloned.GetTruncated(),
		Error:                cloned.GetError(),
		SourceQueryObjectKey: cloned.GetSourceQueryObjectKey(),
		TargetDbObjectKey:    cloned.GetTargetDbObjectKey(),
		RowCount:             cloned.GetRowCount(),
	}, nil
}

var _ SRPCSqlQueryResultResourceServiceServer = (*SqlQueryResultResource)(nil)
