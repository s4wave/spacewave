package resource_block_cursor

import (
	"context"
	"errors"
	"testing"

	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/testbed"
	s4wave_block_cursor "github.com/s4wave/spacewave/sdk/block/cursor"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	"github.com/sirupsen/logrus"
)

const exampleBlockTypeID = "github.com/s4wave/spacewave/db/block/mock.Example"

func TestUnmarshalWithBlockTypeReusesDecodedCursor(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addExampleBlockTypeController(t, ctx, tb)

	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "typed cursor"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	resource := NewBlockCursorResource(le, tb.Bus, tx, cursor)

	opCtx, counter := block.WithReadCounter(ctx)
	resp, err := resource.Unmarshal(opCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed cursor")

	resp, err = resource.Unmarshal(opCtx, &s4wave_block_cursor.UnmarshalRequest{BlockType: exampleBlockTypeID})
	if err != nil {
		t.Fatal(err.Error())
	}
	assertExampleResponse(t, resp.GetData(), "typed cursor")

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 0 ||
		snapshot.DecodedBlockCacheMissCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 1 {
		t.Fatalf("unexpected typed cursor counters: %+v", snapshot)
	}
}

func TestSetBlockWithSqlRootBlockTypesWritesBrowserSeededRoots(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	addSpaceBlockTypeController(t, ctx, tb)

	rawStore := block_mock.NewMockStore(0)
	rawTx, rawCursor := block.NewTransaction(rawStore, nil, nil, nil)
	rawResource := NewBlockCursorResource(le, tb.Bus, rawTx, rawCursor)
	querySeed := &s4wave_sql_query.Query{
		SqlText:           "SELECT name FROM quickstart.people",
		DialectHint:       "mysql",
		TargetDbObjectKey: "sql/db",
	}
	queryData, err := querySeed.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := rawResource.SetBlock(ctx, &s4wave_block_cursor.SetBlockRequest{
		Data:      queryData,
		MarkDirty: true,
	}); err != nil {
		t.Fatal(err.Error())
	}
	if _, _, err := rawTx.Write(ctx, true); !errors.Is(err, block.ErrNotBlock) {
		t.Fatalf("raw SQL root write error = %v, want %v", err, block.ErrNotBlock)
	}

	queryRoot := writeTypedSqlRoot(
		t,
		ctx,
		le,
		tb,
		s4wave_sql_query.SqlQueryBlockTypeID,
		querySeed,
	)
	query, err := block.UnmarshalBlock[*s4wave_sql_query.Query](ctx, queryRoot, s4wave_sql_query.NewQueryBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if query.GetSqlText() != querySeed.GetSqlText() ||
		query.GetDialectHint() != querySeed.GetDialectHint() ||
		query.GetTargetDbObjectKey() != querySeed.GetTargetDbObjectKey() {
		t.Fatalf("query root = %#v, want %#v", query, querySeed)
	}

	workbenchSeed := &s4wave_sql_workbench.Workbench{
		TargetDbObjectKey: "sql/db",
		DisplayName:       "Browser SQL Workbench",
		PinnedQueryObjectKeys: []string{
			"sql/query/example",
			"sql/query/e2e-second",
		},
	}
	workbenchRoot := writeTypedSqlRoot(
		t,
		ctx,
		le,
		tb,
		s4wave_sql_workbench.SqlWorkbenchBlockTypeID,
		workbenchSeed,
	)
	workbench, err := block.UnmarshalBlock[*s4wave_sql_workbench.Workbench](ctx, workbenchRoot, s4wave_sql_workbench.NewWorkbenchBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if workbench.GetTargetDbObjectKey() != workbenchSeed.GetTargetDbObjectKey() ||
		workbench.GetDisplayName() != workbenchSeed.GetDisplayName() ||
		len(workbench.GetPinnedQueryObjectKeys()) != len(workbenchSeed.GetPinnedQueryObjectKeys()) {
		t.Fatalf("workbench root = %#v, want %#v", workbench, workbenchSeed)
	}
}

func addExampleBlockTypeController(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()
	controller := blocktype_controller.NewController(func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == exampleBlockTypeID {
			return blocktype.NewBlockType(exampleBlockTypeID, func() *block_mock.Example {
				return &block_mock.Example{}
			}), nil
		}
		return nil, nil
	})
	release, err := tb.Bus.AddController(ctx, controller, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(release)
}

func addSpaceBlockTypeController(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()
	controller := blocktype_controller.NewController(space_world.LookupBlockType)
	release, err := tb.Bus.AddController(ctx, controller, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(release)
}

func writeTypedSqlRoot(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	tb *testbed.Testbed,
	blockTypeID string,
	root block.Block,
) *block.Cursor {
	t.Helper()
	data, err := root.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	store := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	resource := NewBlockCursorResource(le, tb.Bus, tx, cursor)
	if _, err := resource.SetBlock(ctx, &s4wave_block_cursor.SetBlockRequest{
		Data:      data,
		MarkDirty: true,
		BlockType: blockTypeID,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref, rootCursor, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref.GetEmpty() {
		t.Fatal("typed SQL root write returned empty ref")
	}
	return rootCursor
}

func assertExampleResponse(t *testing.T, data []byte, want string) {
	t.Helper()
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(data); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want)
	}
}
