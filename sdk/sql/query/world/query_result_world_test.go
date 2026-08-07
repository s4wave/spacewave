package s4wave_sql_query_world_test

import (
	"context"
	std_errors "errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
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

func TestSqlQueryInitializeCreatesFirstRootOnce(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	firstDBKey := "sql/query-initialize-test/db-first"
	createSqlDbObject(t, ctx, tb.WorldState, firstDBKey)
	secondDBKey := "sql/query-initialize-test/db-second"
	createSqlDbObject(t, ctx, tb.WorldState, secondDBKey)
	queryKey := "sql/query-initialize-test/query"
	createEmptySqlQueryObject(t, ctx, tb.WorldState, queryKey)
	client, cleanup := openSqlQueryClient(t, ctx, tb, queryKey)
	defer cleanup()

	if _, err := client.Initialize(ctx, &s4wave_sql_query.InitializeQueryRequest{
		SqlText:           "SELECT 1",
		DialectHint:       "mysql",
		TargetDbObjectKey: firstDBKey,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	query, err := client.GetQueryText(ctx, &s4wave_sql_query.GetQueryTextRequest{})
	if err != nil {
		t.Fatalf("GetQueryText: %v", err)
	}
	assertQueryText(t, query, "SELECT 1", "mysql", firstDBKey)
	if len(query.GetParameters()) != 0 {
		t.Fatalf("initialized parameters = %#v, want empty", query.GetParameters())
	}
	assertGraphQuad(t, ctx, tb.WorldState, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), firstDBKey)

	_, err = client.Initialize(ctx, &s4wave_sql_query.InitializeQueryRequest{
		SqlText:           "SELECT 2",
		DialectHint:       "postgres",
		TargetDbObjectKey: secondDBKey,
	})
	if err == nil || err.Error() != s4wave_sql_query_world.ErrQueryAlreadyInitialized.Error() {
		t.Fatalf("duplicate Initialize error = %v, want %q", err, s4wave_sql_query_world.ErrQueryAlreadyInitialized)
	}
	query, err = client.GetQueryText(ctx, &s4wave_sql_query.GetQueryTextRequest{})
	if err != nil {
		t.Fatalf("GetQueryText after duplicate: %v", err)
	}
	assertQueryText(t, query, "SELECT 1", "mysql", firstDBKey)
	assertGraphQuad(t, ctx, tb.WorldState, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), firstDBKey)
	assertNoGraphQuad(t, ctx, tb.WorldState, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), secondDBKey)
}

func TestSqlQueryInitializeAcceptsEmptyTarget(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	queryKey := "sql/query-initialize-empty-target/query"
	createEmptySqlQueryObject(t, ctx, tb.WorldState, queryKey)
	client, cleanup := openSqlQueryClient(t, ctx, tb, queryKey)
	defer cleanup()

	if _, err := client.Initialize(ctx, &s4wave_sql_query.InitializeQueryRequest{
		SqlText:     "SELECT 1",
		DialectHint: "sqlite",
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	query, err := client.GetQueryText(ctx, &s4wave_sql_query.GetQueryTextRequest{})
	if err != nil {
		t.Fatalf("GetQueryText: %v", err)
	}
	assertQueryText(t, query, "SELECT 1", "sqlite", "")
	if len(query.GetParameters()) != 0 {
		t.Fatalf("initialized parameters = %#v, want empty", query.GetParameters())
	}
	assertNoGraphQuad(t, ctx, tb.WorldState, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), "")
}

func TestSqlQueryInitializeRejectsInvalidTarget(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	invalidTargetKey := "sql/query-initialize-invalid-target/not-db"
	createEmptySqlQueryObject(t, ctx, tb.WorldState, invalidTargetKey)
	queryKey := "sql/query-initialize-invalid-target/query"
	createEmptySqlQueryObject(t, ctx, tb.WorldState, queryKey)
	client, cleanup := openSqlQueryClient(t, ctx, tb, queryKey)
	defer cleanup()

	if _, err := client.Initialize(ctx, &s4wave_sql_query.InitializeQueryRequest{
		SqlText:           "SELECT 1",
		TargetDbObjectKey: invalidTargetKey,
	}); err == nil {
		t.Fatal("Initialize accepted a non-sql/db target")
	}
	assertObjectRootEmpty(t, ctx, tb.WorldState, queryKey)
}

func TestSqlQueryInitializeWorldOpCreateOnce(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	queryKey := "sql/query-initialize-op/query"
	createEmptySqlQueryObject(t, ctx, tb.WorldState, queryKey)
	obj, err := world.MustGetObject(ctx, tb.WorldState, queryKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)

	firstRef := writeQueryRootRef(t, ctx, tb.WorldState, &s4wave_sql_query.Query{SqlText: "SELECT 1"})
	if _, err := s4wave_sql_query_world.NewSqlQueryInitializeRootOp(queryKey, firstRef).
		ApplyWorldObjectOp(ctx, nil, obj, ""); err != nil {
		t.Fatalf("first initialize-only op: %v", err)
	}
	assertQueryRoot(t, ctx, tb.WorldState, queryKey, "SELECT 1")

	secondRef := writeQueryRootRef(t, ctx, tb.WorldState, &s4wave_sql_query.Query{SqlText: "SELECT 2"})
	_, err = s4wave_sql_query_world.NewSqlQueryInitializeRootOp(queryKey, secondRef).
		ApplyWorldObjectOp(ctx, nil, obj, "")
	if !std_errors.Is(err, s4wave_sql_query_world.ErrQueryAlreadyInitialized) {
		t.Fatalf("second initialize-only op error = %v, want %v", err, s4wave_sql_query_world.ErrQueryAlreadyInitialized)
	}
	if err.Error() != "sql/query: already initialized" {
		t.Fatalf("second initialize-only op error = %q, want stable message", err)
	}
	assertQueryRoot(t, ctx, tb.WorldState, queryKey, "SELECT 1")

	if _, err := s4wave_sql_query_world.NewSqlQuerySetRootOp(queryKey, secondRef).
		ApplyWorldObjectOp(ctx, nil, obj, ""); err != nil {
		t.Fatalf("ordinary set-root op: %v", err)
	}
	assertQueryRoot(t, ctx, tb.WorldState, queryKey, "SELECT 2")
}

func TestSqlQueryInitializeConcurrentWinnerRetriesStaleGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	raceEngine := newQueryInitializeRaceEngine(tb.Engine)
	ws := world.NewEngineWorldState(raceEngine, true)
	dbKey := "sql/query-initialize-race/db"
	createSqlDbObject(t, ctx, ws, dbKey)
	queryKey := "sql/query-initialize-race/query"
	createEmptySqlQueryObject(t, ctx, ws, queryKey)
	resource := s4wave_sql_query_world.NewSqlQueryResource(ws, raceEngine, queryKey)
	raceEngine.EnableStaleCommitRace()

	requests := []*s4wave_sql_query.InitializeQueryRequest{
		{SqlText: "SELECT 1", DialectHint: "mysql", TargetDbObjectKey: dbKey},
		{SqlText: "SELECT 2", DialectHint: "postgres", TargetDbObjectKey: dbKey},
	}
	errs := make([]error, len(requests))
	var wg sync.WaitGroup
	for i, req := range requests {
		wg.Go(func() {
			_, errs[i] = resource.Initialize(ctx, req)
		})
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner != -1 {
				t.Fatalf("Initialize errors = %v, want exactly one success", errs)
			}
			winner = i
			continue
		}
		if !std_errors.Is(err, s4wave_sql_query_world.ErrQueryAlreadyInitialized) {
			t.Fatalf("Initialize[%d] error = %v, want %v", i, err, s4wave_sql_query_world.ErrQueryAlreadyInitialized)
		}
	}
	if winner == -1 {
		t.Fatalf("Initialize errors = %v, want one success", errs)
	}
	commits, staleCommits := raceEngine.CommitCounts()
	if commits != 2 || staleCommits != 1 {
		t.Fatalf("commit attempts = %d, stale = %d, want 2 and 1", commits, staleCommits)
	}
	query, err := s4wave_sql_query.ReadQueryRoot(ctx, ws, queryKey)
	if err != nil {
		t.Fatalf("ReadQueryRoot: %v", err)
	}
	winnerReq := requests[winner]
	if query.GetSqlText() != winnerReq.GetSqlText() ||
		query.GetDialectHint() != winnerReq.GetDialectHint() ||
		query.GetTargetDbObjectKey() != winnerReq.GetTargetDbObjectKey() {
		t.Fatalf("final query = %#v, want winner request %#v", query, winnerReq)
	}
	assertGraphQuad(t, ctx, ws, queryKey, s4wave_sql.PredSqlQueryAgainst.String(), dbKey)
}

type queryInitializeRaceEngine struct {
	world.Engine

	mu              sync.Mutex
	enabled         bool
	nextWriteTx     int
	commits         int
	staleCommits    int
	winnerCommitted chan struct{}
}

func newQueryInitializeRaceEngine(engine world.Engine) *queryInitializeRaceEngine {
	return &queryInitializeRaceEngine{Engine: engine}
}

func (e *queryInitializeRaceEngine) EnableStaleCommitRace() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = true
	e.nextWriteTx = 0
	e.commits = 0
	e.staleCommits = 0
	e.winnerCommitted = make(chan struct{})
}

func (e *queryInitializeRaceEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	var sequence int
	if e.enabled && write {
		e.nextWriteTx++
		sequence = e.nextWriteTx
	}
	e.mu.Unlock()
	return &queryInitializeRaceTx{Tx: tx, engine: e, sequence: sequence}, nil
}

func (e *queryInitializeRaceEngine) CommitCounts() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.commits, e.staleCommits
}

type queryInitializeRaceTx struct {
	world.Tx
	engine   *queryInitializeRaceEngine
	sequence int
}

func (tx *queryInitializeRaceTx) Commit(ctx context.Context) error {
	tx.engine.mu.Lock()
	enabled := tx.engine.enabled
	if enabled {
		tx.engine.commits++
	}
	winnerCommitted := tx.engine.winnerCommitted
	tx.engine.mu.Unlock()
	if !enabled {
		return tx.Tx.Commit(ctx)
	}

	switch tx.sequence {
	case 1:
		tx.Discard()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-winnerCommitted:
		}
		tx.engine.mu.Lock()
		tx.engine.staleCommits++
		tx.engine.mu.Unlock()
		return coord.ErrStaleGeneration
	case 2:
		err := tx.Tx.Commit(ctx)
		close(winnerCommitted)
		return err
	default:
		return tx.Tx.Commit(ctx)
	}
}

func assertQueryText(
	t *testing.T,
	query *s4wave_sql_query.GetQueryTextResponse,
	sqlText string,
	dialectHint string,
	targetDBKey string,
) {
	t.Helper()
	if query.GetSqlText() != sqlText ||
		query.GetDialectHint() != dialectHint ||
		query.GetTargetDbObjectKey() != targetDBKey {
		t.Fatalf("query = %#v, want sql=%q dialect=%q target=%q", query, sqlText, dialectHint, targetDBKey)
	}
}

func assertObjectRootEmpty(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rootRef != nil && !rootRef.GetRootRef().GetEmpty() {
		t.Fatalf("object %q root = %#v, want empty", objectKey, rootRef)
	}
}

func writeQueryRootRef(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	query *s4wave_sql_query.Query,
) *bucket.ObjectRef {
	t.Helper()
	rootRef, err := s4wave_sql_query.WriteQueryRootRef(ctx, ws, query)
	if err != nil {
		t.Fatalf("WriteQueryRootRef: %v", err)
	}
	return rootRef
}

func assertQueryRoot(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	wantSQL string,
) {
	t.Helper()
	query, err := s4wave_sql_query.ReadQueryRoot(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("ReadQueryRoot: %v", err)
	}
	if query.GetSqlText() != wantSQL {
		t.Fatalf("query SQL = %q, want %q", query.GetSqlText(), wantSQL)
	}
}

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

func createEmptySqlQueryObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	obj, err := ws.CreateObject(ctx, objectKey, &bucket.ObjectRef{})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("CreateObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
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

func assertNoGraphQuad(
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
	if len(quads) != 0 {
		t.Fatalf("LookupGraphQuads(%s, %s, %s) returned %d quads, want 0", subject, predicate, object, len(quads))
	}
}
