//go:build !tinygo && !sql_lite

package s4wave_sql_workbench_world_test

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/sirupsen/logrus"
)

func TestSqlWorkbenchPinsPersistAcrossEngineReopen(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := db_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	eng := openSqlWorkbenchTestEngine(t, ctx, le, tb, nil)
	ws := world.NewEngineWorldState(eng, true)

	dbKey := "sql/workbench-test/db"
	queryKey := "sql/workbench-test/query"
	removedQueryKey := "sql/workbench-test/query-removed"
	resultKey := "sql/workbench-test/result"
	workbenchKey := "sql/workbench-test/workbench"

	createSqlDbObject(t, ctx, ws, dbKey)
	createSqlQueryObject(t, ctx, ws, queryKey, &s4wave_sql_query.Query{
		SqlText:           "SELECT 1",
		TargetDbObjectKey: dbKey,
	})
	createSqlQueryObject(t, ctx, ws, removedQueryKey, &s4wave_sql_query.Query{
		SqlText:           "SELECT 2",
		TargetDbObjectKey: dbKey,
	})
	createSqlQueryResultObject(t, ctx, ws, resultKey, &s4wave_sql_query_result.QueryResult{
		SourceQueryObjectKey: queryKey,
		TargetDbObjectKey:    dbKey,
	})
	createSqlWorkbenchObject(t, ctx, ws, workbenchKey, &s4wave_sql_workbench.Workbench{
		TargetDbObjectKey: dbKey,
		DisplayName:       "Main SQL Workbench",
	})

	client, cleanup := openSqlWorkbenchClient(t, ctx, ws, workbenchKey)
	defer cleanup()

	if _, err := client.AddPin(ctx, &s4wave_sql_workbench.AddPinRequest{QueryObjectKey: queryKey}); err != nil {
		t.Fatalf("AddPin(%s): %v", queryKey, err)
	}
	if _, err := client.AddPin(ctx, &s4wave_sql_workbench.AddPinRequest{QueryObjectKey: queryKey}); err != nil {
		t.Fatalf("AddPin duplicate(%s): %v", queryKey, err)
	}
	if _, err := client.AddPin(ctx, &s4wave_sql_workbench.AddPinRequest{QueryObjectKey: removedQueryKey}); err != nil {
		t.Fatalf("AddPin(%s): %v", removedQueryKey, err)
	}
	if _, err := client.RemovePin(ctx, &s4wave_sql_workbench.RemovePinRequest{QueryObjectKey: removedQueryKey}); err != nil {
		t.Fatalf("RemovePin(%s): %v", removedQueryKey, err)
	}
	if _, err := client.SetLayout(ctx, &s4wave_sql_workbench.SetLayoutRequest{
		OpenTabs: []*s4wave_sql_workbench.WorkbenchTab{
			{
				TabId:     "query",
				ObjectKey: queryKey,
				Kind:      s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY,
				Title:     "Query",
				Pinned:    true,
			},
			{
				TabId:     "result",
				ObjectKey: resultKey,
				Kind:      s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY_RESULT,
				Title:     "Result",
			},
		},
		Layout: &s4wave_sql_workbench.WorkbenchLayout{
			Mode:              "split",
			SidebarWidth:      280,
			ResultPanelHeight: 420,
			ActiveTabId:       "query",
		},
	}); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}

	beforeReopen, err := client.GetWorkbench(ctx, &s4wave_sql_workbench.GetWorkbenchRequest{})
	if err != nil {
		t.Fatalf("GetWorkbench before reopen: %v", err)
	}
	assertWorkbenchState(t, beforeReopen.GetWorkbench(), dbKey, queryKey, resultKey)
	assertGraphQuad(t, ctx, ws, workbenchKey, s4wave_sql.PredSqlWorkbenchAgainstDb.String(), dbKey)
	assertGraphQuad(t, ctx, ws, workbenchKey, s4wave_sql.PredSqlWorkbenchPinnedQuery.String(), queryKey)
	assertGraphQuad(t, ctx, ws, workbenchKey, s4wave_sql.PredSqlWorkbenchOpenTab.String(), queryKey)
	assertGraphQuad(t, ctx, ws, workbenchKey, s4wave_sql.PredSqlWorkbenchOpenTab.String(), resultKey)

	persistedRoot := eng.GetRootRef().Clone()
	if err := eng.Close(); err != nil {
		t.Fatalf("close engine before reopen: %v", err)
	}

	reopened := openSqlWorkbenchTestEngine(t, ctx, le, tb, persistedRoot)
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("close reopened engine: %v", err)
		}
	}()
	reopenedWS := world.NewEngineWorldState(reopened, true)
	reopenedClient, reopenedCleanup := openSqlWorkbenchClient(t, ctx, reopenedWS, workbenchKey)
	defer reopenedCleanup()

	afterReopen, err := reopenedClient.GetWorkbench(ctx, &s4wave_sql_workbench.GetWorkbenchRequest{})
	if err != nil {
		t.Fatalf("GetWorkbench after reopen: %v", err)
	}
	assertWorkbenchState(t, afterReopen.GetWorkbench(), dbKey, queryKey, resultKey)
	assertGraphQuad(t, ctx, reopenedWS, workbenchKey, s4wave_sql.PredSqlWorkbenchPinnedQuery.String(), queryKey)
}

func openSqlWorkbenchTestEngine(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	tb *db_testbed.Testbed,
	rootRef *bucket.ObjectRef,
) *world_block.Engine {
	t.Helper()
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatalf("BuildEmptyCursor: %v", err)
	}
	if rootRef != nil {
		cursor.SetRootRef(rootRef.GetRootRef())
	}
	eng, err := world_block.NewEngine(ctx, le, cursor, optypes.LookupWorldOp, nil, false)
	if err != nil {
		cursor.Release()
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
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

func createSqlQueryObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	query *s4wave_sql_query.Query,
) {
	t.Helper()
	_, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(query, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
	_, sysErr, err := ws.ApplyWorldOp(ctx, s4wave_sql_query_world.NewSqlQuerySetRootOp(objectKey, rootRef), "")
	if err != nil || sysErr {
		t.Fatalf("SqlQuerySetRootOp sysErr=%v err=%v", sysErr, err)
	}
}

func createSqlQueryResultObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	result *s4wave_sql_query_result.QueryResult,
) {
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(result, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func createSqlWorkbenchObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	workbench *s4wave_sql_workbench.Workbench,
) {
	t.Helper()
	_, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(workbench, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_workbench.SqlWorkbenchTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
	_, sysErr, err := ws.ApplyWorldOp(ctx, s4wave_sql_workbench_world.NewSqlWorkbenchSetRootOp(objectKey, rootRef), "")
	if err != nil || sysErr {
		t.Fatalf("SqlWorkbenchSetRootOp sysErr=%v err=%v", sysErr, err)
	}
}

func openSqlWorkbenchClient(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (s4wave_sql_workbench.SRPCSqlWorkbenchResourceServiceClient, func()) {
	t.Helper()
	inv, cleanup, err := s4wave_sql_workbench_world.SqlWorkbenchFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		nil,
		nil,
		ws,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlWorkbenchFactory: %v", err)
	}
	client := s4wave_sql_workbench.NewSRPCSqlWorkbenchResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))),
	)
	return client, cleanup
}

func assertWorkbenchState(
	t *testing.T,
	workbench *s4wave_sql_workbench.Workbench,
	dbKey string,
	queryKey string,
	resultKey string,
) {
	t.Helper()
	if workbench.GetTargetDbObjectKey() != dbKey {
		t.Fatalf("target db key = %q, want %q", workbench.GetTargetDbObjectKey(), dbKey)
	}
	pins := workbench.GetPinnedQueryObjectKeys()
	if !slices.Equal(pins, []string{queryKey}) {
		t.Fatalf("pinned query keys = %#v, want [%q]", pins, queryKey)
	}
	tabs := workbench.GetOpenTabs()
	if len(tabs) != 2 {
		t.Fatalf("open tabs = %d, want 2", len(tabs))
	}
	if tabs[0].GetObjectKey() != queryKey || tabs[0].GetKind() != s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY {
		t.Fatalf("query tab = %#v, want object %q kind query", tabs[0], queryKey)
	}
	if tabs[1].GetObjectKey() != resultKey || tabs[1].GetKind() != s4wave_sql_workbench.WorkbenchTabKind_WORKBENCH_TAB_KIND_QUERY_RESULT {
		t.Fatalf("result tab = %#v, want object %q kind query-result", tabs[1], resultKey)
	}
	layout := workbench.GetLayout()
	if layout.GetMode() != "split" || layout.GetActiveTabId() != "query" {
		t.Fatalf("layout = %#v, want split active query", layout)
	}
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
