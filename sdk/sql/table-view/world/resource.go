//go:build !tinygo && !sql_lite

package s4wave_sql_table_view_world

import (
	"context"
	"database/sql/driver"
	std_errors "errors"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

const fetchRowsBatchSize = 128

// SqlTableViewResource serves SqlTableViewResourceService for one SQL table view object.
type SqlTableViewResource struct {
	ws        world.WorldState
	objectKey string
	mux       srpc.Mux
}

// NewSqlTableViewResource constructs a SQL table view resource.
func NewSqlTableViewResource(
	ws world.WorldState,
	objectKey string,
) *SqlTableViewResource {
	r := &SqlTableViewResource{
		ws:        ws,
		objectKey: objectKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_sql_table_view.SRPCRegisterSqlTableViewResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *SqlTableViewResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the resource lifecycle.
func (r *SqlTableViewResource) Close() {}

// GetTableView returns the table view metadata.
func (r *SqlTableViewResource) GetTableView(
	ctx context.Context,
	_ *s4wave_sql_table_view.GetTableViewRequest,
) (*s4wave_sql_table_view.GetTableViewResponse, error) {
	tableView, err := r.readTableView(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_sql_table_view.GetTableViewResponse{TableView: tableView.CloneVT()}, nil
}

// FetchRows executes the table view SELECT.
func (r *SqlTableViewResource) FetchRows(
	ctx context.Context,
	_ *s4wave_sql_table_view.FetchRowsRequest,
) (*s4wave_sql_table_view.FetchRowsResponse, error) {
	tableView, err := r.readTableView(ctx)
	if err != nil {
		return nil, err
	}
	schema, err := r.readTargetSchema(ctx, tableView.GetTargetSchemaObjectKey())
	if err != nil {
		return nil, err
	}
	if err := world_types.CheckObjectType(ctx, r.ws, schema.GetTargetDbObjectKey(), s4wave_sql_world.SqlDbTypeID); err != nil {
		return nil, err
	}
	query, args, maxRows, err := compileTableViewSelect(schema, tableView)
	if err != nil {
		return nil, err
	}
	rows, cleanup, err := r.openTargetRows(ctx, schema.GetTargetDbObjectKey(), query, args)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return readFetchRows(rows, maxRows)
}

func (r *SqlTableViewResource) readTableView(ctx context.Context) (*s4wave_sql_table_view.TableView, error) {
	if r.ws == nil {
		return nil, errors.New("sql/table-view: world state is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, s4wave_sql_table_view.SqlTableViewTypeID); err != nil {
		return nil, err
	}
	return s4wave_sql_table_view.ReadTableViewRoot(ctx, r.ws, r.objectKey)
}

func (r *SqlTableViewResource) readTargetSchema(
	ctx context.Context,
	schemaKey string,
) (*s4wave_sql_schema.Schema, error) {
	if schemaKey == "" {
		return nil, errors.New("sql/table-view: target schema object key is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, schemaKey, s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		return nil, err
	}
	schema, err := s4wave_sql_schema.ReadSchemaRoot(ctx, r.ws, schemaKey)
	if err != nil {
		return nil, err
	}
	if schema.GetTargetDbObjectKey() == "" {
		return nil, errors.New("sql/table-view: target schema database object key is required")
	}
	return schema, nil
}

func (r *SqlTableViewResource) openTargetRows(
	ctx context.Context,
	targetKey string,
	query string,
	args []driver.NamedValue,
) (driver.Rows, func(), error) {
	obj, err := world.MustGetObject(ctx, r.ws, targetKey)
	if err != nil {
		return nil, nil, err
	}
	var store *s4wave_sql_world.WorldBackedSql
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = s4wave_sql_world.NewWorldBackedSql(ctx, root.Clone(), r.ws, targetKey)
		return err
	}); err != nil {
		return nil, nil, err
	}
	tx, err := store.NewSqlTransaction(ctx, false, "")
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		tx.Discard()
		store.Close()
		return nil, nil, err
	}
	rows, err := ops.QueryContext(ctx, query, args)
	if std_errors.Is(err, driver.ErrSkip) {
		rows, err = ops.Query(query, sql_rpc.NamedValuesToValues(args))
	}
	if err != nil {
		tx.Discard()
		store.Close()
		return nil, nil, err
	}
	if rows == nil {
		tx.Discard()
		store.Close()
		return nil, nil, errors.New("sql/table-view: fetch rows returned nil rows")
	}
	cleanup := func() {
		rows.Close()
		tx.Discard()
		store.Close()
	}
	return rows, cleanup, nil
}

func readFetchRows(rows driver.Rows, maxRows uint32) (*s4wave_sql_table_view.FetchRowsResponse, error) {
	columns := rows.Columns()
	columnTypes, _ := rows.(driver.RowsColumnTypeDatabaseTypeName)
	resp := &s4wave_sql_table_view.FetchRowsResponse{
		Columns: make([]*hydra_sql.ColumnSchema, len(columns)),
	}
	for i, name := range columns {
		resp.Columns[i] = &hydra_sql.ColumnSchema{Name: name}
		if columnTypes != nil {
			resp.Columns[i].DatabaseTypeName = columnTypes.ColumnTypeDatabaseTypeName(i)
		}
	}

	dest := make([]driver.Value, len(columns))
	batch := &hydra_sql.RowBatch{}
	for resp.GetRowCount() < uint64(maxRows) {
		clear(dest)
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		row, err := rowFromDriverValues(dest)
		if err != nil {
			return nil, err
		}
		batch.Rows = append(batch.Rows, row)
		resp.RowCount++
		if len(batch.GetRows()) == fetchRowsBatchSize {
			resp.RowBatches = append(resp.RowBatches, batch)
			batch = &hydra_sql.RowBatch{}
		}
	}
	if len(batch.GetRows()) != 0 {
		resp.RowBatches = append(resp.RowBatches, batch)
	}
	clear(dest)
	err := rows.Next(dest)
	if err == nil {
		resp.Truncated = true
		return resp, nil
	}
	if err != io.EOF {
		return nil, err
	}
	return resp, nil
}

func rowFromDriverValues(values []driver.Value) (*hydra_sql.Row, error) {
	row := &hydra_sql.Row{Values: make([]*hydra_sql.SqlValue, len(values))}
	for i, value := range values {
		wireValue, err := sql_rpc.DriverValueToSqlValue(value)
		if err != nil {
			return nil, err
		}
		row.Values[i] = wireValue
	}
	return row, nil
}

// _ is a type assertion.
var _ s4wave_sql_table_view.SRPCSqlTableViewResourceServiceServer = (*SqlTableViewResource)(nil)
