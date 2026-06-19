//go:build !tinygo && !sql_lite

package s4wave_sql_schema_world

import (
	"context"
	"database/sql/driver"
	std_errors "errors"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

// SqlSchemaResource serves SqlSchemaResourceService for one SQL schema object.
type SqlSchemaResource struct {
	ws        world.WorldState
	objectKey string
	mux       srpc.Mux
}

// NewSqlSchemaResource constructs a SQL schema resource.
func NewSqlSchemaResource(
	ws world.WorldState,
	objectKey string,
) *SqlSchemaResource {
	r := &SqlSchemaResource{
		ws:        ws,
		objectKey: objectKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_sql_schema.SRPCRegisterSqlSchemaResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *SqlSchemaResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the resource lifecycle.
func (r *SqlSchemaResource) Close() {}

// GetSchema returns the schema metadata.
func (r *SqlSchemaResource) GetSchema(
	ctx context.Context,
	_ *s4wave_sql_schema.GetSchemaRequest,
) (*s4wave_sql_schema.GetSchemaResponse, error) {
	schema, err := r.readSchema(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_sql_schema.GetSchemaResponse{Schema: schema.CloneVT()}, nil
}

// ListTables lists tables in the target sql/db schema.
func (r *SqlSchemaResource) ListTables(
	ctx context.Context,
	_ *s4wave_sql_schema.ListTablesRequest,
) (*s4wave_sql_schema.ListTablesResponse, error) {
	schema, err := r.readSchema(ctx)
	if err != nil {
		return nil, err
	}
	targetKey := schema.GetTargetDbObjectKey()
	if targetKey == "" {
		return nil, errors.New("sql/schema: target database object key is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, targetKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		return nil, err
	}
	query, err := listTablesSQL(schema.GetSchemaName())
	if err != nil {
		return nil, err
	}
	rows, cleanup, err := r.openTargetRows(ctx, targetKey, query)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return readTableInfoRows(rows)
}

func (r *SqlSchemaResource) readSchema(ctx context.Context) (*s4wave_sql_schema.Schema, error) {
	if r.ws == nil {
		return nil, errors.New("sql/schema: world state is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		return nil, err
	}
	return s4wave_sql_schema.ReadSchemaRoot(ctx, r.ws, r.objectKey)
}

func listTablesSQL(schemaName string) (string, error) {
	schemaIdent, err := s4wave_sql.QuoteIdentifier(schemaName)
	if err != nil {
		return "", err
	}
	return "SHOW TABLES FROM " + schemaIdent, nil
}

func (r *SqlSchemaResource) openTargetRows(
	ctx context.Context,
	targetKey string,
	query string,
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
	rows, err := ops.QueryContext(ctx, query, nil)
	if std_errors.Is(err, driver.ErrSkip) {
		rows, err = ops.Query(query, nil)
	}
	if err != nil {
		tx.Discard()
		store.Close()
		return nil, nil, err
	}
	if rows == nil {
		tx.Discard()
		store.Close()
		return nil, nil, errors.New("sql/schema: list tables returned nil rows")
	}
	cleanup := func() {
		rows.Close()
		tx.Discard()
		store.Close()
	}
	return rows, cleanup, nil
}

func readTableInfoRows(rows driver.Rows) (*s4wave_sql_schema.ListTablesResponse, error) {
	if len(rows.Columns()) == 0 {
		return nil, errors.New("sql/schema: list tables returned no columns")
	}
	resp := &s4wave_sql_schema.ListTablesResponse{}
	dest := make([]driver.Value, len(rows.Columns()))
	for {
		clear(dest)
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				return resp, nil
			}
			return nil, err
		}
		name, err := tableNameFromValue(dest[0])
		if err != nil {
			return nil, err
		}
		resp.Tables = append(resp.Tables, &s4wave_sql_schema.TableInfo{Name: name})
	}
}

func tableNameFromValue(value driver.Value) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", errors.Errorf("sql/schema: unsupported table name value %T", value)
	}
}

// _ is a type assertion.
var _ s4wave_sql_schema.SRPCSqlSchemaResourceServiceServer = (*SqlSchemaResource)(nil)
