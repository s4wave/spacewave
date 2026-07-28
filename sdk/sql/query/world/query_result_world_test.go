package s4wave_sql_query_world_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestSqlQueryRunCreatesLinkedQueryResult(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	dbKey := "sql/query-test/db"
	createSqlDbObject(t, ctx, tb.WorldState, dbKey)
	seedSqlDb(t, ctx, tb, dbKey)

	queryKey := "sql/query-test/query"
	createSqlQueryObject(t, ctx, tb.WorldState, queryKey)
	queryClient, queryCleanup := openSqlQueryClient(t, ctx, tb, queryKey)
	defer queryCleanup()

	if _, err := queryClient.SetQueryText(ctx, &s4wave_sql_query.SetQueryTextRequest{
		SqlText:           "SELECT name FROM alpha.people WHERE id = ?",
		DialectHint:       "mysql",
		TargetDbObjectKey: dbKey,
	}); err != nil {
		t.Fatalf("SetQueryText: %v", err)
	}
	if _, err := queryClient.SetParameters(ctx, &s4wave_sql_query.SetParametersRequest{
		Parameters: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 1}},
		},
	}); err != nil {
		t.Fatalf("SetParameters: %v", err)
	}
	queryText, err := queryClient.GetQueryText(ctx, &s4wave_sql_query.GetQueryTextRequest{})
	if err != nil {
		t.Fatalf("GetQueryText: %v", err)
	}
	if queryText.GetTargetDbObjectKey() != dbKey {
		t.Fatalf("target db = %q, want %q", queryText.GetTargetDbObjectKey(), dbKey)
	}
	params := queryText.GetParameters()
	if len(params) != 1 {
		t.Fatalf("parameters = %d, want 1", len(params))
	}
	param, ok := params[0].GetValue().(*hydra_sql.SqlValue_IntValue)
	if !ok || param.IntValue != 1 {
		t.Fatalf("parameter[0] = %#v, want int 1", params[0])
	}
	assertGraphQuad(t, ctx, tb.WorldState, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), dbKey)

	runResp, err := queryClient.Run(ctx, &s4wave_sql_query.RunQueryRequest{MaxRows: 16})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runResp.GetError() != "" {
		t.Fatalf("Run returned SQL error: %s", runResp.GetError())
	}
	resultKey := runResp.GetResultObjectKey()
	if resultKey == "" {
		t.Fatal("Run returned empty result object key")
	}
	if err := world_types.CheckObjectType(ctx, tb.WorldState, resultKey, s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
		t.Fatalf("result object type: %v", err)
	}

	resultClient, resultCleanup := openSqlQueryResultClient(t, ctx, tb, resultKey)
	defer resultCleanup()
	grid, err := resultClient.GetResultGrid(ctx, &s4wave_sql_query_result.GetResultGridRequest{})
	if err != nil {
		t.Fatalf("GetResultGrid: %v", err)
	}
	if grid.GetSourceQueryObjectKey() != queryKey {
		t.Fatalf("source query key = %q, want %q", grid.GetSourceQueryObjectKey(), queryKey)
	}
	if grid.GetTargetDbObjectKey() != dbKey {
		t.Fatalf("target db key = %q, want %q", grid.GetTargetDbObjectKey(), dbKey)
	}
	if grid.GetRowCount() != 1 {
		t.Fatalf("row count = %d, want 1", grid.GetRowCount())
	}
	if len(grid.GetColumns()) != 1 || grid.GetColumns()[0].GetName() != "name" {
		t.Fatalf("columns = %#v, want one name column", grid.GetColumns())
	}
	value := singleResultValue(t, grid.GetRowBatches())
	if value != "ada" {
		t.Fatalf("result value = %q, want ada", value)
	}
	assertGraphQuad(t, ctx, tb.WorldState, resultKey, s4wave_sql.PredSqlQueryProducedBy.String(), queryKey)
	assertGraphQuad(t, ctx, tb.WorldState, resultKey, s4wave_sql.PredSqlQueryAgainst.String(), dbKey)
}

func createSqlDbObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func createSqlQueryObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&s4wave_sql_query.Query{}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func seedSqlDb(t *testing.T, ctx context.Context, tb *testbed.Testbed, objectKey string) {
	t.Helper()
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory: %v", err)
	}
	defer cleanup()

	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSql(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	for _, query := range []string{
		"CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO people (id, name) VALUES (1, 'ada')",
	} {
		execSql(t, ctx, writeTx, query)
	}
	commitSql(t, ctx, writeTx)
}

func openSqlQueryClient(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	objectKey string,
) (s4wave_sql_query.SRPCSqlQueryResourceServiceClient, func()) {
	t.Helper()
	inv, cleanup, err := s4wave_sql_query_world.SqlQueryFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlQueryFactory: %v", err)
	}
	client := s4wave_sql_query.NewSRPCSqlQueryResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))))
	return client, cleanup
}

func openSqlQueryResultClient(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	objectKey string,
) (s4wave_sql_query_result.SRPCSqlQueryResultResourceServiceClient, func()) {
	t.Helper()
	inv, cleanup, err := s4wave_sql_query_result_world.SqlQueryResultFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlQueryResultFactory: %v", err)
	}
	client := s4wave_sql_query_result.NewSRPCSqlQueryResultResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))),
	)
	return client, cleanup
}

func openSqlTx(
	t *testing.T,
	ctx context.Context,
	store hydra_sql.SqlStore,
	write bool,
	dsn string,
) hydra_sql.SqlTransaction {
	t.Helper()
	tx, err := store.NewSqlTransaction(ctx, write, dsn)
	if err != nil {
		t.Fatalf("NewSqlTransaction(write=%v, dsn=%q): %v", write, dsn, err)
	}
	return tx
}

func execSql(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction, query string) {
	t.Helper()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		tx.Discard()
		t.Fatalf("GetSqlOps: %v", err)
	}
	if _, err := ops.ExecContext(ctx, query, nil); err != nil {
		tx.Discard()
		t.Fatalf("%s: %v", query, err)
	}
}

func commitSql(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction) {
	t.Helper()
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatalf("Commit: %v", err)
	}
}

func singleResultValue(t *testing.T, batches []*hydra_sql.RowBatch) string {
	t.Helper()
	if len(batches) != 1 {
		t.Fatalf("row batches = %d, want 1", len(batches))
	}
	rows := batches[0].GetRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	values := rows[0].GetValues()
	if len(values) != 1 {
		t.Fatalf("values = %d, want 1", len(values))
	}
	switch value := values[0].GetValue().(type) {
	case *hydra_sql.SqlValue_StrValue:
		return value.StrValue
	case *hydra_sql.SqlValue_BlobValue:
		return string(value.BlobValue)
	default:
		t.Fatalf("value = %#v, want string/blob", values[0])
	}
	return ""
}

func assertGraphQuad(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	subject string,
	predicate string,
	object string,
) {
	t.Helper()
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(subject, predicate, object, ""), 0)
	if err != nil {
		t.Fatalf("LookupGraphQuads(%s, %s, %s): %v", subject, predicate, object, err)
	}
	if len(quads) != 1 {
		t.Fatalf("LookupGraphQuads(%s, %s, %s) returned %d quads, want 1", subject, predicate, object, len(quads))
	}
}
