//go:build !tinygo

package s4wave_sql_query_world

import (
	"context"
	"database/sql/driver"
	std_errors "errors"
	"io"
	"strconv"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

const (
	defaultRunMaxRows  uint32 = 1_000
	resultRowBatchSize        = 128
)

// SqlQueryResource serves SqlQueryResourceService for one SQL query object.
type SqlQueryResource struct {
	ws        world.WorldState
	engine    world.Engine
	objectKey string
	mux       srpc.Mux
}

// NewSqlQueryResource constructs a SQL query resource.
func NewSqlQueryResource(
	ws world.WorldState,
	engine world.Engine,
	objectKey string,
) *SqlQueryResource {
	r := &SqlQueryResource{
		ws:        ws,
		engine:    engine,
		objectKey: objectKey,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_sql_query.SRPCRegisterSqlQueryResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *SqlQueryResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the resource lifecycle.
func (r *SqlQueryResource) Close() {}

// Initialize creates the first query root.
func (r *SqlQueryResource) Initialize(
	ctx context.Context,
	req *s4wave_sql_query.InitializeQueryRequest,
) (*s4wave_sql_query.InitializeQueryResponse, error) {
	if r.ws == nil {
		return nil, errors.New("sql/query: world state is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return nil, err
	}
	if targetKey := req.GetTargetDbObjectKey(); targetKey != "" {
		if err := world_types.CheckObjectType(ctx, r.ws, targetKey, s4wave_sql_world.SqlDbTypeID); err != nil {
			return nil, err
		}
	}
	query := &s4wave_sql_query.Query{
		SqlText:           req.GetSqlText(),
		DialectHint:       req.GetDialectHint(),
		TargetDbObjectKey: req.GetTargetDbObjectKey(),
	}
	rootRef, err := s4wave_sql_query.WriteQueryRootRef(ctx, r.ws, query)
	if err != nil {
		return nil, err
	}
	_, sysErr, err := r.ws.ApplyWorldOp(ctx, NewSqlQueryInitializeRootOp(r.objectKey, rootRef), "")
	if err != nil {
		return nil, err
	}
	if sysErr {
		return nil, errors.New("sql/query: root initialization returned a system error")
	}
	return &s4wave_sql_query.InitializeQueryResponse{}, nil
}

// GetQueryText reads the query text and target metadata.
func (r *SqlQueryResource) GetQueryText(
	ctx context.Context,
	_ *s4wave_sql_query.GetQueryTextRequest,
) (*s4wave_sql_query.GetQueryTextResponse, error) {
	query, err := r.readQuery(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_sql_query.GetQueryTextResponse{
		SqlText:           query.GetSqlText(),
		DialectHint:       query.GetDialectHint(),
		TargetDbObjectKey: query.GetTargetDbObjectKey(),
		Parameters:        cloneSqlValues(query.GetParameters()),
	}, nil
}

// SetQueryText updates query text and target metadata.
func (r *SqlQueryResource) SetQueryText(
	ctx context.Context,
	req *s4wave_sql_query.SetQueryTextRequest,
) (*s4wave_sql_query.SetQueryTextResponse, error) {
	query, err := r.readQuery(ctx)
	if err != nil {
		return nil, err
	}
	if targetKey := req.GetTargetDbObjectKey(); targetKey != "" {
		if err := world_types.CheckObjectType(ctx, r.ws, targetKey, s4wave_sql_world.SqlDbTypeID); err != nil {
			return nil, err
		}
	}
	next := query.CloneVT()
	next.SqlText = req.GetSqlText()
	next.DialectHint = req.GetDialectHint()
	next.TargetDbObjectKey = req.GetTargetDbObjectKey()
	if err := r.commitQueryRoot(ctx, next); err != nil {
		return nil, err
	}
	return &s4wave_sql_query.SetQueryTextResponse{}, nil
}

// SetParameters updates positional bind arguments.
func (r *SqlQueryResource) SetParameters(
	ctx context.Context,
	req *s4wave_sql_query.SetParametersRequest,
) (*s4wave_sql_query.SetParametersResponse, error) {
	query, err := r.readQuery(ctx)
	if err != nil {
		return nil, err
	}
	next := query.CloneVT()
	next.Parameters = cloneSqlValues(req.GetParameters())
	if err := r.commitQueryRoot(ctx, next); err != nil {
		return nil, err
	}
	return &s4wave_sql_query.SetParametersResponse{}, nil
}

// Run executes the query and creates a linked query result object.
func (r *SqlQueryResource) Run(
	ctx context.Context,
	req *s4wave_sql_query.RunQueryRequest,
) (*s4wave_sql_query.RunQueryResponse, error) {
	if r.engine == nil {
		return nil, errors.New("sql/query: resource is read-only")
	}
	query, err := r.readQuery(ctx)
	if err != nil {
		return nil, err
	}
	targetKey := query.GetTargetDbObjectKey()
	if targetKey == "" {
		return nil, errors.New("sql/query: target database object key is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, targetKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := &s4wave_sql_query_result.QueryResult{
		ExecutedAt:           timestamppb.New(now),
		SourceQueryObjectKey: r.objectKey,
		TargetDbObjectKey:    targetKey,
	}
	r.executeQuery(ctx, query, req.GetMaxRows(), result)

	resultKey, err := r.createResultObject(ctx, result, now)
	if err != nil {
		return nil, err
	}
	resp := &s4wave_sql_query.RunQueryResponse{ResultObjectKey: resultKey}
	if result.GetError() != nil {
		resp.Error = result.GetError().GetMessage()
	}
	return resp, nil
}

func (r *SqlQueryResource) readQuery(ctx context.Context) (*s4wave_sql_query.Query, error) {
	if r.ws == nil {
		return nil, errors.New("sql/query: world state is required")
	}
	if err := world_types.CheckObjectType(ctx, r.ws, r.objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return nil, err
	}
	return s4wave_sql_query.ReadQueryRoot(ctx, r.ws, r.objectKey)
}

func (r *SqlQueryResource) commitQueryRoot(ctx context.Context, query *s4wave_sql_query.Query) error {
	rootRef, err := s4wave_sql_query.WriteQueryRootRef(ctx, r.ws, query)
	if err != nil {
		return err
	}
	_, sysErr, err := r.ws.ApplyWorldOp(ctx, NewSqlQuerySetRootOp(r.objectKey, rootRef), "")
	if err != nil {
		return err
	}
	if sysErr {
		return errors.New("sql/query: root update returned a system error")
	}
	return nil
}

func (r *SqlQueryResource) executeQuery(
	ctx context.Context,
	query *s4wave_sql_query.Query,
	maxRows uint32,
	result *s4wave_sql_query_result.QueryResult,
) {
	if maxRows == 0 {
		maxRows = defaultRunMaxRows
	}
	rows, cleanup, err := r.openRows(ctx, query)
	if err != nil {
		result.Error = &s4wave_sql_query_result.QueryResultError{Message: err.Error()}
		return
	}
	defer cleanup()

	columns := rows.Columns()
	columnTypes, _ := rows.(driver.RowsColumnTypeDatabaseTypeName)
	result.Columns = make([]*hydra_sql.ColumnSchema, len(columns))
	for i, name := range columns {
		result.Columns[i] = &hydra_sql.ColumnSchema{Name: name}
		if columnTypes != nil {
			result.Columns[i].DatabaseTypeName = columnTypes.ColumnTypeDatabaseTypeName(i)
		}
	}

	dest := make([]driver.Value, len(columns))
	batch := &hydra_sql.RowBatch{}
	for result.GetRowCount() < uint64(maxRows) {
		clear(dest)
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				break
			}
			result.Error = &s4wave_sql_query_result.QueryResultError{Message: err.Error()}
			break
		}
		row := &hydra_sql.Row{Values: make([]*hydra_sql.SqlValue, len(dest))}
		for i, value := range dest {
			wireValue, err := sql_rpc.DriverValueToSqlValue(value)
			if err != nil {
				result.Error = &s4wave_sql_query_result.QueryResultError{Message: err.Error()}
				return
			}
			row.Values[i] = wireValue
		}
		batch.Rows = append(batch.Rows, row)
		result.RowCount++
		if len(batch.GetRows()) == resultRowBatchSize {
			result.RowBatches = append(result.RowBatches, batch)
			batch = &hydra_sql.RowBatch{}
		}
	}
	if len(batch.GetRows()) != 0 {
		result.RowBatches = append(result.RowBatches, batch)
	}
	if result.GetError() != nil {
		return
	}
	clear(dest)
	err = rows.Next(dest)
	if err == nil {
		result.Truncated = true
		return
	}
	if err != io.EOF {
		result.Error = &s4wave_sql_query_result.QueryResultError{Message: err.Error()}
	}
}

func (r *SqlQueryResource) openRows(
	ctx context.Context,
	query *s4wave_sql_query.Query,
) (driver.Rows, func(), error) {
	targetKey := query.GetTargetDbObjectKey()
	obj, err := world.MustGetObject(ctx, r.ws, targetKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sql/query: open target db object")
	}
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sql/query: read target db root")
	}
	storageRoot, err := r.engine.BuildStorageCursor(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sql/query: open target db storage cursor")
	}
	root, err := storageRoot.FollowRef(ctx, rootRef)
	if err != nil {
		storageRoot.Release()
		return nil, nil, errors.Wrap(err, "sql/query: follow target db root")
	}
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, root, r.ws, targetKey)
	if err != nil {
		root.Release()
		storageRoot.Release()
		return nil, nil, errors.Wrap(err, "sql/query: open target db store")
	}
	tx, err := store.NewSqlTransaction(ctx, false, "")
	if err != nil {
		store.Close()
		storageRoot.Release()
		return nil, nil, errors.Wrap(err, "sql/query: open target db read transaction")
	}
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		tx.Discard()
		store.Close()
		storageRoot.Release()
		return nil, nil, errors.Wrap(err, "sql/query: open target db SQL ops")
	}
	args := sql_rpc.SqlValuesToNamedValues(query.GetParameters())
	rows, err := ops.QueryContext(ctx, query.GetSqlText(), args)
	if std_errors.Is(err, driver.ErrSkip) {
		rows, err = ops.Query(query.GetSqlText(), sql_rpc.NamedValuesToValues(args))
	}
	if err != nil {
		tx.Discard()
		store.Close()
		storageRoot.Release()
		return nil, nil, errors.Wrap(err, "sql/query: execute target db query")
	}
	if rows == nil {
		tx.Discard()
		store.Close()
		storageRoot.Release()
		return nil, nil, errors.New("sql/query: query returned nil rows")
	}
	cleanup := func() {
		rows.Close()
		tx.Discard()
		store.Close()
		storageRoot.Release()
	}
	return rows, cleanup, nil
}

func (r *SqlQueryResource) createResultObject(
	ctx context.Context,
	result *s4wave_sql_query_result.QueryResult,
	now time.Time,
) (string, error) {
	baseKey := r.objectKey + "/results/" + strconv.FormatInt(now.UnixNano(), 10)
	var lastErr error
	for attempt := range 8 {
		resultKey := baseKey
		if attempt != 0 {
			resultKey = baseKey + "-" + strconv.Itoa(attempt)
		}
		err := r.createResultObjectAtKey(ctx, resultKey, result)
		if err == nil {
			return resultKey, nil
		}
		lastErr = err
		if !std_errors.Is(err, world.ErrObjectExists) {
			return "", err
		}
	}
	return "", lastErr
}

func (r *SqlQueryResource) createResultObjectAtKey(
	ctx context.Context,
	resultKey string,
	result *s4wave_sql_query_result.QueryResult,
) error {
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if err := world_types.CheckObjectType(ctx, wtx, r.objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		wtx.Discard()
		return err
	}
	if err := world_types.CheckObjectType(ctx, wtx, result.GetTargetDbObjectKey(), s4wave_sql_world.SqlDbTypeID); err != nil {
		wtx.Discard()
		return err
	}
	_, _, err = world.CreateWorldObject(ctx, wtx, resultKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(result, true)
		return nil
	})
	if err != nil {
		wtx.Discard()
		return err
	}
	if err := world_types.SetObjectType(ctx, wtx, resultKey, s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
		wtx.Discard()
		return err
	}
	if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		resultKey,
		s4wave_sql.PredSqlQueryProducedBy.String(),
		r.objectKey,
		"",
	)); err != nil {
		wtx.Discard()
		return err
	}
	if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		resultKey,
		s4wave_sql.PredSqlQueryAgainst.String(),
		result.GetTargetDbObjectKey(),
		"",
	)); err != nil {
		wtx.Discard()
		return err
	}
	if err := wtx.Commit(ctx); err != nil {
		wtx.Discard()
		return err
	}
	return nil
}

func cloneSqlValues(values []*hydra_sql.SqlValue) []*hydra_sql.SqlValue {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]*hydra_sql.SqlValue, len(values))
	for i, value := range values {
		if value != nil {
			cloned[i] = value.CloneVT()
		}
	}
	return cloned
}

// _ is a type assertion.
var _ s4wave_sql_query.SRPCSqlQueryResourceServiceServer = (*SqlQueryResource)(nil)
