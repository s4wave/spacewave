package sql_plugin

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestLookupSQLBlockTypeOwnsSQLCursorBlockTypes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		typeID     string
		wantID     string
		wantMatch  func(block.Block) bool
		wantAbsent bool
	}{
		{
			name:      "query root",
			typeID:    s4wave_sql_query.SqlQueryBlockTypeID,
			wantID:    s4wave_sql_query.SqlQueryBlockTypeID,
			wantMatch: s4wave_sql_query.SqlQueryBlockType.MatchesBlockType,
		},
		{
			name:      "workbench root",
			typeID:    s4wave_sql_workbench.SqlWorkbenchBlockTypeID,
			wantID:    s4wave_sql_workbench.SqlWorkbenchBlockTypeID,
			wantMatch: s4wave_sql_workbench.SqlWorkbenchBlockType.MatchesBlockType,
		},
		{
			name:       "object type id is not a block type",
			typeID:     s4wave_sql_world.SqlDbTypeID,
			wantAbsent: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lookupSQLBlockType(tc.typeID)
			if err != nil {
				t.Fatalf("lookupSQLBlockType(%s): %v", tc.typeID, err)
			}
			if tc.wantAbsent {
				if got != nil {
					t.Fatalf("lookupSQLBlockType(%s) = %T, want nil", tc.typeID, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("lookupSQLBlockType(%s) returned nil", tc.typeID)
			}
			if got.GetBlockTypeID() != tc.wantID {
				t.Fatalf("block type id = %q, want %q", got.GetBlockTypeID(), tc.wantID)
			}
			blk := got.Constructor()
			if !tc.wantMatch(blk) {
				t.Fatalf("constructor returned %T, not a block matching %s", blk, tc.wantID)
			}
		})
	}
}

func TestSQLHandlerValidateOpUnmarshalsEverySQLOperation(t *testing.T) {
	rootRef := testSQLObjectRef(t)
	for _, tc := range []struct {
		name string
		op   world.Operation
	}{
		{
			name: "sql db set root",
			op:   s4wave_sql_world.NewSqlSetRootOp("sql/db/test", nil, rootRef, nil),
		},
		{
			name: "query set root",
			op:   s4wave_sql_query_world.NewSqlQuerySetRootOp("sql/query/test", rootRef),
		},
		{
			name: "query result set root",
			op:   s4wave_sql_query_result_world.NewSqlQueryResultSetRootOp("sql/query-result/test", rootRef),
		},
		{
			name: "schema set root",
			op:   s4wave_sql_schema_world.NewSqlSchemaSetRootOp("sql/schema/test", rootRef),
		},
		{
			name: "table view set root",
			op:   s4wave_sql_table_view_world.NewSqlTableViewSetRootOp("sql/table-view/test", rootRef),
		},
		{
			name: "workbench set root",
			op:   s4wave_sql_workbench_world.NewSqlWorkbenchSetRootOp("sql/workbench/test", rootRef),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.op.MarshalBlock()
			if err != nil {
				t.Fatalf("MarshalBlock: %v", err)
			}

			h := &SQLHandler{}
			resp, err := h.ValidateOp(context.Background(), &s4wave_worldop_registry.ValidateOpRequest{
				OperationTypeId: tc.op.GetOperationTypeId(),
				OpData:          data,
			})
			if err != nil {
				t.Fatalf("ValidateOp returned RPC error: %v", err)
			}
			if resp.GetError() != "" {
				t.Fatalf("ValidateOp(%s) error = %q, want empty", tc.op.GetOperationTypeId(), resp.GetError())
			}

			decoded, err := h.unmarshalSQLOp(context.Background(), tc.op.GetOperationTypeId(), data)
			if err != nil {
				t.Fatalf("unmarshalSQLOp(%s): %v", tc.op.GetOperationTypeId(), err)
			}
			if decoded.GetOperationTypeId() != tc.op.GetOperationTypeId() {
				t.Fatalf("decoded op id = %q, want %q", decoded.GetOperationTypeId(), tc.op.GetOperationTypeId())
			}
			if err := decoded.Validate(); err != nil {
				t.Fatalf("decoded op Validate: %v", err)
			}
		})
	}
}

func TestSQLHandlerValidateOpReportsLookupUnmarshalAndValidationErrors(t *testing.T) {
	h := &SQLHandler{}
	for _, tc := range []struct {
		name    string
		opID    string
		data    []byte
		wantErr string
	}{
		{
			name:    "unknown operation type",
			opID:    "sql/missing/set-root",
			wantErr: "unhandled operation type",
		},
		{
			name:    "invalid protobuf payload",
			opID:    s4wave_sql_world.SqlSetRootOpId,
			data:    []byte{0xff},
			wantErr: "unexpected EOF",
		},
		{
			name:    "decoded op fails Validate",
			opID:    s4wave_sql_world.SqlSetRootOpId,
			wantErr: "object key cannot be empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := h.ValidateOp(context.Background(), &s4wave_worldop_registry.ValidateOpRequest{
				OperationTypeId: tc.opID,
				OpData:          tc.data,
			})
			if err != nil {
				t.Fatalf("ValidateOp returned RPC error: %v", err)
			}
			if !strings.Contains(resp.GetError(), tc.wantErr) {
				t.Fatalf("ValidateOp error = %q, want substring %q", resp.GetError(), tc.wantErr)
			}
		})
	}
}

func TestRegisterSQLRejectsZeroResourceIDs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		zeroAt  string
		wantErr string
	}{
		{
			name:    "object type registry",
			zeroAt:  "object",
			wantErr: "sql plugin: object type sql/db registration returned zero resource id",
		},
		{
			name:    "world op registry",
			zeroAt:  "worldop",
			wantErr: "sql plugin: world op sql/db/set-root registration returned zero resource id",
		},
		{
			name:    "quickstart registry",
			zeroAt:  "quickstart",
			wantErr: "sql plugin: quickstart sql registration returned zero resource id",
		},
		{
			name:    "viewer registry",
			zeroAt:  "viewer",
			wantErr: "sql plugin: viewer sql/db registration returned zero resource id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldViewers := sqlViewerRegistrations
			sqlViewerRegistrations = []*s4wave_viewer_registry.ViewerRegistration{{
				TypeId:     s4wave_sql_world.SqlDbTypeID,
				ViewerName: "SQL DB",
				ScriptPath: "plugin/sql/viewer.tsx",
			}}
			t.Cleanup(func() { sqlViewerRegistrations = oldViewers })

			fake := &zeroResourceRegistry{zeroAt: tc.zeroAt, nextID: 1}
			client, rootClient, cleanup := newRegisterSQLHarness(t, fake)
			defer cleanup()

			refs, err := (&Controller{}).registerSQL(context.Background(), client, rootClient)
			defer releaseRefs(refs)
			if err == nil {
				t.Fatal("registerSQL succeeded, want zero resource id error")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("registerSQL error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSQLHandlerSeedSQLQuickstartCreatesReadableDatabaseAndExampleQuery(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	handler := &SQLHandler{
		le: logrus.NewEntry(logrus.New()),
		b:  tb.Bus,
	}
	if err := handler.seedSQLQuickstart(ctx, tb.WorldState); err != nil {
		t.Fatalf("seedSQLQuickstart: %v", err)
	}

	if err := world_types.CheckObjectType(ctx, tb.WorldState, sqlQuickstartDBKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("%s object type: %v", sqlQuickstartDBKey, err)
	}

	store, cleanupStore := openQuickstartSQLStore(t, ctx, handler, tb)
	defer cleanupStore()
	rootTx := openQuickstartSQLTx(t, ctx, store, false, "")
	defer rootTx.Discard()
	schemas := queryQuickstartStringColumn(t, ctx, rootTx, "SHOW DATABASES")
	if !containsString(schemas, "quickstart") {
		t.Fatalf("SHOW DATABASES = %v, want quickstart", schemas)
	}

	row := queryQuickstartSingleRow(t, ctx, rootTx, "SELECT name, role FROM quickstart.people WHERE id = 1", []string{"name", "role"})
	if row[0] != "ada" || row[1] != "analyst" {
		t.Fatalf("quickstart.people id=1 = %v, want [ada analyst]", row)
	}

	project := queryQuickstartSingleRow(t, ctx, rootTx, "SELECT title FROM quickstart.projects WHERE id = 10", []string{"title"})
	if project[0] != "difference engine notes" {
		t.Fatalf("quickstart.projects id=10 = %v, want [difference engine notes]", project)
	}

	if err := world_types.CheckObjectType(ctx, tb.WorldState, sqlQuickstartQueryKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		t.Fatalf("%s object type: %v", sqlQuickstartQueryKey, err)
	}
	queryClient, cleanupQuery := openQuickstartQueryClient(t, ctx, handler, tb)
	defer cleanupQuery()
	query, err := queryClient.GetQueryText(ctx, &s4wave_sql_query.GetQueryTextRequest{})
	if err != nil {
		t.Fatalf("GetQueryText(%s): %v", sqlQuickstartQueryKey, err)
	}
	if got := query.GetTargetDbObjectKey(); got != sqlQuickstartDBKey {
		t.Fatalf("%s target db = %q, want %q", sqlQuickstartQueryKey, got, sqlQuickstartDBKey)
	}
}

func TestSQLObjectTypeBridgeAccessTypedObjectReadsQuickstartSchema(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	handler := &SQLHandler{
		le: le,
		b:  tb.Bus,
	}
	if err := handler.seedSQLQuickstart(ctx, tb.WorldState); err != nil {
		t.Fatalf("seedSQLQuickstart: %v", err)
	}

	engineClient, typedObjects, cleanupBridge := newSQLObjectTypeBridgeHarness(
		t,
		ctx,
		le,
		handler,
		tb,
		s4wave_sql_world.SqlDbTypeID,
	)
	defer cleanupBridge()
	accessResp, err := typedObjects.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: sqlQuickstartDBKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject(%s): %v", sqlQuickstartDBKey, err)
	}
	if accessResp.GetTypeId() != s4wave_sql_world.SqlDbTypeID {
		t.Fatalf("AccessTypedObject(%s) type = %q, want %q", sqlQuickstartDBKey, accessResp.GetTypeId(), s4wave_sql_world.SqlDbTypeID)
	}

	sqlRef := engineClient.CreateResourceReference(accessResp.GetResourceId())
	defer sqlRef.Release()
	sqlClient, err := sqlRef.GetClient()
	if err != nil {
		t.Fatalf("SQL typed resource client: %v", err)
	}
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(sqlClient))
	tx := openQuickstartSQLTx(t, ctx, store, false, "")
	defer tx.Discard()
	schemas := queryQuickstartStringColumn(t, ctx, tx, "SHOW DATABASES")
	if !containsString(schemas, "quickstart") {
		t.Fatalf("SHOW DATABASES through bridged SQL resource = %v, want quickstart", schemas)
	}
}

func TestSQLObjectTypeBridgeRunQuickstartQueryServesResultGrid(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	handler := &SQLHandler{
		le: le,
		b:  tb.Bus,
	}
	if err := handler.seedSQLQuickstart(ctx, tb.WorldState); err != nil {
		t.Fatalf("seedSQLQuickstart: %v", err)
	}

	engineClient, typedObjects, cleanupBridge := newSQLObjectTypeBridgeHarness(
		t,
		ctx,
		le,
		handler,
		tb,
		s4wave_sql_world.SqlDbTypeID,
		s4wave_sql_query.SqlQueryTypeID,
		s4wave_sql_query_result.SqlQueryResultTypeID,
	)
	defer cleanupBridge()

	queryAccess, err := typedObjects.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: sqlQuickstartQueryKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject(%s): %v", sqlQuickstartQueryKey, err)
	}
	if queryAccess.GetTypeId() != s4wave_sql_query.SqlQueryTypeID {
		t.Fatalf("AccessTypedObject(%s) type = %q, want %q", sqlQuickstartQueryKey, queryAccess.GetTypeId(), s4wave_sql_query.SqlQueryTypeID)
	}
	queryRef := engineClient.CreateResourceReference(queryAccess.GetResourceId())
	defer queryRef.Release()
	querySRPC, err := queryRef.GetClient()
	if err != nil {
		t.Fatalf("SQL query typed resource client: %v", err)
	}
	queryClient := s4wave_sql_query.NewSRPCSqlQueryResourceServiceClient(querySRPC)
	runResp, err := queryClient.Run(ctx, &s4wave_sql_query.RunQueryRequest{MaxRows: 8})
	if err != nil {
		t.Fatalf("Run(%s): %v", sqlQuickstartQueryKey, err)
	}
	if runResp.GetError() != "" {
		t.Fatalf("Run(%s) returned SQL error: %s", sqlQuickstartQueryKey, runResp.GetError())
	}
	resultKey := runResp.GetResultObjectKey()
	if resultKey == "" {
		t.Fatal("Run returned empty result object key")
	}

	resultAccess, err := typedObjects.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: resultKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject(%s): %v", resultKey, err)
	}
	if resultAccess.GetTypeId() != s4wave_sql_query_result.SqlQueryResultTypeID {
		t.Fatalf("AccessTypedObject(%s) type = %q, want %q", resultKey, resultAccess.GetTypeId(), s4wave_sql_query_result.SqlQueryResultTypeID)
	}
	resultRef := engineClient.CreateResourceReference(resultAccess.GetResourceId())
	defer resultRef.Release()
	resultSRPC, err := resultRef.GetClient()
	if err != nil {
		t.Fatalf("SQL query result typed resource client: %v", err)
	}
	resultClient := s4wave_sql_query_result.NewSRPCSqlQueryResultResourceServiceClient(resultSRPC)
	grid, err := resultClient.GetResultGrid(ctx, &s4wave_sql_query_result.GetResultGridRequest{})
	if err != nil {
		t.Fatalf("GetResultGrid(%s): %v", resultKey, err)
	}
	if grid.GetError() != nil {
		t.Fatalf("GetResultGrid(%s) returned SQL error: %s", resultKey, grid.GetError().GetMessage())
	}
	if grid.GetSourceQueryObjectKey() != sqlQuickstartQueryKey {
		t.Fatalf("source query key = %q, want %q", grid.GetSourceQueryObjectKey(), sqlQuickstartQueryKey)
	}
	if grid.GetTargetDbObjectKey() != sqlQuickstartDBKey {
		t.Fatalf("target db key = %q, want %q", grid.GetTargetDbObjectKey(), sqlQuickstartDBKey)
	}
	if grid.GetRowCount() != 1 {
		t.Fatalf("row count = %d, want 1", grid.GetRowCount())
	}
	if got := len(grid.GetColumns()); got != 2 {
		t.Fatalf("columns = %d, want 2", got)
	}
	if grid.GetColumns()[0].GetName() != "name" || grid.GetColumns()[1].GetName() != "role" {
		t.Fatalf("columns = [%s %s], want [name role]", grid.GetColumns()[0].GetName(), grid.GetColumns()[1].GetName())
	}
	row := singleQuickstartResultRow(t, grid.GetRowBatches(), 2)
	if row[0] != "ada" || row[1] != "analyst" {
		t.Fatalf("result row = %v, want [ada analyst]", row)
	}
}

func openQuickstartSQLStore(
	t *testing.T,
	ctx context.Context,
	handler *SQLHandler,
	tb *testbed.Testbed,
) (hydra_sql.SqlStore, func()) {
	t.Helper()
	inv, cleanup, err := handler.openObjectType(
		ctx,
		s4wave_sql_world.SqlDbTypeID,
		tb.BusEngine,
		tb.WorldState,
		sqlQuickstartDBKey,
	)
	if err != nil {
		t.Fatalf("openObjectType(%s): %v", sqlQuickstartDBKey, err)
	}
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	return store, cleanup
}

func openQuickstartQueryClient(
	t *testing.T,
	ctx context.Context,
	handler *SQLHandler,
	tb *testbed.Testbed,
) (s4wave_sql_query.SRPCSqlQueryResourceServiceClient, func()) {
	t.Helper()
	inv, cleanup, err := handler.openObjectType(
		ctx,
		s4wave_sql_query.SqlQueryTypeID,
		tb.BusEngine,
		tb.WorldState,
		sqlQuickstartQueryKey,
	)
	if err != nil {
		t.Fatalf("openObjectType(%s): %v", sqlQuickstartQueryKey, err)
	}
	client := s4wave_sql_query.NewSRPCSqlQueryResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))))
	return client, cleanup
}

func newSQLObjectTypeBridgeHarness(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	handler *SQLHandler,
	tb *testbed.Testbed,
	typeIDs ...string,
) (*resource_client.Client, s4wave_world.SRPCTypedObjectResourceServiceClient, func()) {
	t.Helper()

	var cleanupFns []func()
	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	registry := resource_objecttype_registry.NewObjectTypeRegistryResource()
	registryClient, registryRootClient, cleanupRegistry := newSQLResourceClient(t, registry.GetMux())
	cleanupFns = append(cleanupFns, cleanupRegistry)
	otRegistry := s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceClient(registryRootClient)
	for _, typeID := range typeIDs {
		regResp, err := otRegistry.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
			TypeId:   typeID,
			PluginId: PluginID,
		})
		if err != nil {
			cleanup()
			t.Fatalf("RegisterObjectType(%s): %v", typeID, err)
		}
		if regResp.GetResourceId() == 0 {
			cleanup()
			t.Fatalf("RegisterObjectType(%s) returned zero resource id", typeID)
		}
		regRef := registryClient.CreateResourceReference(regResp.GetResourceId())
		cleanupFns = append(cleanupFns, regRef.Release)
	}

	pluginClient := newSQLPluginClient(t, handler)
	pluginRel, err := tb.Bus.AddController(ctx, &sqlPluginLoadTestController{client: pluginClient}, nil)
	if err != nil {
		cleanup()
		t.Fatalf("AddController(plugin load): %v", err)
	}
	cleanupFns = append(cleanupFns, pluginRel)

	bridgeRel, err := tb.Bus.AddController(ctx, resource_objecttype_registry.NewBridgeController(le, tb.Bus, registry), nil)
	if err != nil {
		cleanup()
		t.Fatalf("AddController(object type bridge): %v", err)
	}
	cleanupFns = append(cleanupFns, bridgeRel)

	engineResource := resource_world.NewEngineResource(le, tb.Bus, tb.BusEngine, nil, nil)
	engineClient, engineRootClient, cleanupEngine := newSQLResourceClient(t, engineResource.GetMux())
	cleanupFns = append(cleanupFns, cleanupEngine)
	typedObjects := s4wave_world.NewSRPCTypedObjectResourceServiceClient(engineRootClient)
	return engineClient, typedObjects, cleanup
}

func newSQLPluginClient(t *testing.T, handler *SQLHandler) srpc.Client {
	t.Helper()
	rootMux := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(rootMux, handler); err != nil {
		t.Fatalf("register SQL object type handler: %v", err)
	}
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(serverMux); err != nil {
		t.Fatalf("register SQL plugin resource server: %v", err)
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux)))
}

func newSQLResourceClient(t *testing.T, root srpc.Invoker) (*resource_client.Client, srpc.Client, func()) {
	t.Helper()
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := resource_client.NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("new resource client: %v", err)
	}
	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		t.Fatalf("root resource client: %v", err)
	}
	return client, rootClient, func() {
		rootRef.Release()
		client.Release()
	}
}

func openQuickstartSQLTx(
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

func queryQuickstartStringColumn(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction, query string) []string {
	t.Helper()
	rows := queryQuickstartRows(t, ctx, tx, query)
	defer rows.Close()
	if cols := rows.Columns(); len(cols) != 1 {
		t.Fatalf("%s columns = %v, want one column", query, cols)
	}
	var values []string
	for {
		dest := make([]driver.Value, 1)
		if err := rows.Next(dest); err != nil {
			if errors.Is(err, io.EOF) {
				return values
			}
			t.Fatalf("%s next: %v", query, err)
		}
		values = append(values, quickstartDriverString(t, query, dest[0]))
	}
}

func queryQuickstartSingleRow(
	t *testing.T,
	ctx context.Context,
	tx hydra_sql.SqlTransaction,
	query string,
	wantColumns []string,
) []string {
	t.Helper()
	rows := queryQuickstartRows(t, ctx, tx, query)
	defer rows.Close()
	if got := rows.Columns(); !equalStringSlices(got, wantColumns) {
		t.Fatalf("%s columns = %v, want %v", query, got, wantColumns)
	}
	dest := make([]driver.Value, len(wantColumns))
	if err := rows.Next(dest); err != nil {
		t.Fatalf("%s first row: %v", query, err)
	}
	if err := rows.Next(make([]driver.Value, len(wantColumns))); !errors.Is(err, io.EOF) {
		t.Fatalf("%s second row = %v, want EOF", query, err)
	}
	got := make([]string, len(dest))
	for idx, value := range dest {
		got[idx] = quickstartDriverString(t, query, value)
	}
	return got
}

func queryQuickstartRows(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction, query string) driver.Rows {
	t.Helper()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		t.Fatalf("GetSqlOps: %v", err)
	}
	rows, err := ops.QueryContext(ctx, query, nil)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return rows
}

func quickstartDriverString(t *testing.T, query string, value driver.Value) string {
	t.Helper()
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		t.Fatalf("%s value = %#v, want string", query, value)
	}
	return ""
}

func singleQuickstartResultRow(t *testing.T, batches []*hydra_sql.RowBatch, wantValues int) []string {
	t.Helper()
	if len(batches) != 1 {
		t.Fatalf("row batches = %d, want 1", len(batches))
	}
	rows := batches[0].GetRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	values := rows[0].GetValues()
	if len(values) != wantValues {
		t.Fatalf("values = %d, want %d", len(values), wantValues)
	}
	got := make([]string, len(values))
	for idx, value := range values {
		got[idx] = quickstartSQLValueString(t, value)
	}
	return got
}

func quickstartSQLValueString(t *testing.T, value *hydra_sql.SqlValue) string {
	t.Helper()
	switch typed := value.GetValue().(type) {
	case *hydra_sql.SqlValue_StrValue:
		return typed.StrValue
	case *hydra_sql.SqlValue_BlobValue:
		return string(typed.BlobValue)
	default:
		t.Fatalf("value = %#v, want string/blob", value)
	}
	return ""
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func equalStringSlices(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func testSQLObjectRef(t *testing.T) *bucket.ObjectRef {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte("sql plugin test root"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	return &bucket.ObjectRef{RootRef: ref}
}

func newRegisterSQLHarness(t *testing.T, fake *zeroResourceRegistry) (*resource_client.Client, srpc.Client, func()) {
	t.Helper()

	rootMux := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeRegistryResourceService(rootMux, fake); err != nil {
		t.Fatalf("register object type registry: %v", err)
	}
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpRegistryResourceService(rootMux, fake); err != nil {
		t.Fatalf("register world op registry: %v", err)
	}
	if err := s4wave_quickstart_registry.SRPCRegisterQuickstartRegistryResourceService(rootMux, fake); err != nil {
		t.Fatalf("register quickstart registry: %v", err)
	}
	if err := s4wave_viewer_registry.SRPCRegisterViewerRegistryResourceService(rootMux, fake); err != nil {
		t.Fatalf("register viewer registry: %v", err)
	}

	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := resource_client.NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("new resource client: %v", err)
	}
	rootRef := client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		client.Release()
		t.Fatalf("root resource client: %v", err)
	}
	return client, rootClient, func() {
		rootRef.Release()
		client.Release()
	}
}

type sqlPluginLoadTestController struct {
	client srpc.Client
}

func (*sqlPluginLoadTestController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("plugin/sql/test-load", controller.MustParseVersion("0.0.1"), "SQL plugin test loader")
}

func (*sqlPluginLoadTestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *sqlPluginLoadTestController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != PluginID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(c.client)}), nil)
}

func (*sqlPluginLoadTestController) Close() error {
	return nil
}

type zeroResourceRegistry struct {
	zeroAt string
	nextID uint32
}

func (r *zeroResourceRegistry) nextResourceID(kind string) uint32 {
	if r.zeroAt == kind {
		return 0
	}
	id := r.nextID
	r.nextID++
	return id
}

func (r *zeroResourceRegistry) RegisterObjectType(
	context.Context,
	*s4wave_objecttype_registry.RegisterObjectTypeRequest,
) (*s4wave_objecttype_registry.RegisterObjectTypeResponse, error) {
	return &s4wave_objecttype_registry.RegisterObjectTypeResponse{ResourceId: r.nextResourceID("object")}, nil
}

func (*zeroResourceRegistry) WatchObjectTypes(
	*s4wave_objecttype_registry.WatchObjectTypesRequest,
	s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceService_WatchObjectTypesStream,
) error {
	return errors.New("unexpected WatchObjectTypes")
}

func (r *zeroResourceRegistry) RegisterWorldOp(
	context.Context,
	*s4wave_worldop_registry.RegisterWorldOpRequest,
) (*s4wave_worldop_registry.RegisterWorldOpResponse, error) {
	return &s4wave_worldop_registry.RegisterWorldOpResponse{ResourceId: r.nextResourceID("worldop")}, nil
}

func (*zeroResourceRegistry) WatchWorldOps(
	*s4wave_worldop_registry.WatchWorldOpsRequest,
	s4wave_worldop_registry.SRPCWorldOpRegistryResourceService_WatchWorldOpsStream,
) error {
	return errors.New("unexpected WatchWorldOps")
}

func (r *zeroResourceRegistry) RegisterQuickstart(
	context.Context,
	*s4wave_quickstart_registry.RegisterQuickstartRequest,
) (*s4wave_quickstart_registry.RegisterQuickstartResponse, error) {
	return &s4wave_quickstart_registry.RegisterQuickstartResponse{ResourceId: r.nextResourceID("quickstart")}, nil
}

func (*zeroResourceRegistry) ListQuickstarts(
	context.Context,
	*s4wave_quickstart_registry.ListQuickstartsRequest,
) (*s4wave_quickstart_registry.ListQuickstartsResponse, error) {
	return nil, errors.New("unexpected ListQuickstarts")
}

func (*zeroResourceRegistry) WatchQuickstarts(
	*s4wave_quickstart_registry.WatchQuickstartsRequest,
	s4wave_quickstart_registry.SRPCQuickstartRegistryResourceService_WatchQuickstartsStream,
) error {
	return errors.New("unexpected WatchQuickstarts")
}

func (*zeroResourceRegistry) ExecuteQuickstart(
	context.Context,
	*s4wave_quickstart_registry.ExecuteQuickstartRequest,
) (*s4wave_quickstart_registry.ExecuteQuickstartResponse, error) {
	return nil, errors.New("unexpected ExecuteQuickstart")
}

func (r *zeroResourceRegistry) RegisterViewer(
	context.Context,
	*s4wave_viewer_registry.RegisterViewerRequest,
) (*s4wave_viewer_registry.RegisterViewerResponse, error) {
	return &s4wave_viewer_registry.RegisterViewerResponse{ResourceId: r.nextResourceID("viewer")}, nil
}

func (*zeroResourceRegistry) ListViewers(
	context.Context,
	*s4wave_viewer_registry.ListViewersRequest,
) (*s4wave_viewer_registry.ListViewersResponse, error) {
	return nil, errors.New("unexpected ListViewers")
}

func (*zeroResourceRegistry) WatchViewers(
	*s4wave_viewer_registry.WatchViewersRequest,
	s4wave_viewer_registry.SRPCViewerRegistryResourceService_WatchViewersStream,
) error {
	return errors.New("unexpected WatchViewers")
}
