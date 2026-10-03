package s4wave_sql_table_view_world_test

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
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestSqlSchemaListTablesAndTableViewFetchRows(t *testing.T) {
	// Start the World testbed for schema discovery and filtered row reads.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create and seed the SQL database with the people table.
	dbKey := "sql/schema-view-test/db"
	createSqlDbObject(t, ctx, tb.WorldState, dbKey)
	seedSqlDb(t, ctx, tb, dbKey)

	// Create a schema object and open its resource client.
	schemaKey := "sql/schema-view-test/schema"
	createSqlSchemaObject(t, ctx, tb.WorldState, schemaKey, &s4wave_sql_schema.Schema{
		SchemaName:        "alpha",
		TargetDbObjectKey: dbKey,
		DisplayName:       "Alpha",
	})
	schemaClient, schemaCleanup := openSqlSchemaClient(t, ctx, tb, schemaKey)
	defer schemaCleanup()

	// List the schema's tables through the resource client.
	tables, err := schemaClient.ListTables(ctx, &s4wave_sql_schema.ListTablesRequest{})
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}

	// Verify the people table and its schema-to-database graph relationship.
	if !hasTable(tables.GetTables(), "people") {
		t.Fatalf("tables = %#v, want people", tables.GetTables())
	}
	assertGraphQuad(t, ctx, tb.WorldState, schemaKey, s4wave_sql.PredSqlSchemaInDb.String(), dbKey)

	// Create a filtered table view and open its resource client.
	viewKey := "sql/schema-view-test/table-view"
	createSqlTableViewObject(t, ctx, tb.WorldState, viewKey, &s4wave_sql_table_view.TableView{
		TargetSchemaObjectKey: schemaKey,
		TargetTableName:       "people",
		WhereExpression:       "age > ?",
		ProjectedColumns:      []string{"name"},
		SortOrder: []*s4wave_sql_table_view.SortOrder{
			{ColumnName: "name"},
		},
		RowLimit: 10,
		WhereParameters: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 30}},
		},
		DisplayName: "Adults",
	})
	viewClient, viewCleanup := openSqlTableViewClient(t, ctx, tb, viewKey)
	defer viewCleanup()

	// Fetch the table view's projected rows through its resource client.
	rows, err := viewClient.FetchRows(ctx, &s4wave_sql_table_view.FetchRowsRequest{})
	if err != nil {
		t.Fatalf("FetchRows: %v", err)
	}

	// Verify the projected adult row and the table view's graph relationship.
	if rows.GetTruncated() {
		t.Fatal("FetchRows returned truncated result")
	}
	if rows.GetRowCount() != 1 {
		t.Fatalf("row count = %d, want 1", rows.GetRowCount())
	}
	if len(rows.GetColumns()) != 1 || rows.GetColumns()[0].GetName() != "name" {
		t.Fatalf("columns = %#v, want one name column", rows.GetColumns())
	}
	if value := singleStringValue(t, rows.GetRowBatches()); value != "ada" {
		t.Fatalf("filtered value = %q, want ada", value)
	}
	assertGraphQuad(t, ctx, tb.WorldState, viewKey, s4wave_sql.PredSqlTableViewAgainstSchema.String(), schemaKey)
}

func TestSqlTableViewUpdateRowPersistsTypedValue(t *testing.T) {
	// Start the World testbed for typed table view updates.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create and seed the SQL database with the people table.
	dbKey := "sql/schema-view-update-test/db"
	createSqlDbObject(t, ctx, tb.WorldState, dbKey)
	seedSqlDb(t, ctx, tb, dbKey)

	// Create the schema object targeting the seeded database.
	schemaKey := "sql/schema-view-update-test/schema"
	createSqlSchemaObject(t, ctx, tb.WorldState, schemaKey, &s4wave_sql_schema.Schema{
		SchemaName:        "alpha",
		TargetDbObjectKey: dbKey,
		DisplayName:       "Alpha",
	})

	// Create a table view restricted to the first person's age.
	viewKey := "sql/schema-view-update-test/table-view"
	createSqlTableViewObject(t, ctx, tb.WorldState, viewKey, &s4wave_sql_table_view.TableView{
		TargetSchemaObjectKey: schemaKey,
		TargetTableName:       "people",
		WhereExpression:       "id = ?",
		ProjectedColumns:      []string{"age"},
		RowLimit:              10,
		WhereParameters: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 1}},
		},
		DisplayName: "Person age",
	})
	viewClient, viewCleanup := openSqlTableViewClient(t, ctx, tb, viewKey)
	defer viewCleanup()

	// Query the table view's SQL driver capability.
	capability, err := viewClient.GetDriverCapability(ctx, &s4wave_sql_table_view.GetDriverCapabilityRequest{})
	if err != nil {
		t.Fatalf("GetDriverCapability: %v", err)
	}

	// Verify that the driver supports row updates.
	if got := capability.GetCapability().GetUpdateRow(); !got {
		t.Fatalf("update row capability = %v, reason = %q", got, capability.GetCapability().GetUpdateRowUnsupportedReason())
	}

	// Update the first person's age through the table view.
	updateResp, err := viewClient.UpdateRow(ctx, &s4wave_sql_table_view.UpdateRowRequest{
		MatchColumns: []string{"id"},
		MatchValues: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 1}},
		},
		SetColumns: []string{"age"},
		SetValues: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 37}},
		},
	})
	if err != nil {
		t.Fatalf("UpdateRow: %v", err)
	}

	// Verify that the update affected exactly one row.
	if updateResp.GetRowsAffected() != 1 {
		t.Fatalf("rows affected = %d, want 1", updateResp.GetRowsAffected())
	}

	// Attempt an update to a person excluded by the table view filter.
	blockedResp, err := viewClient.UpdateRow(ctx, &s4wave_sql_table_view.UpdateRowRequest{
		MatchColumns: []string{"id"},
		MatchValues: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 2}},
		},
		SetColumns: []string{"age"},
		SetValues: []*hydra_sql.SqlValue{
			{Value: &hydra_sql.SqlValue_IntValue{IntValue: 99}},
		},
	})
	if err != nil {
		t.Fatalf("UpdateRow outside view filter: %v", err)
	}

	// Verify that the table view filter prevented the second update.
	if blockedResp.GetRowsAffected() != 0 {
		t.Fatalf("outside-filter rows affected = %d, want 0", blockedResp.GetRowsAffected())
	}

	// Fetch the table view after updating the person's age.
	rows, err := viewClient.FetchRows(ctx, &s4wave_sql_table_view.FetchRowsRequest{})
	if err != nil {
		t.Fatalf("FetchRows: %v", err)
	}

	// Verify that the table view returns the persisted typed age.
	if rows.GetRowCount() != 1 {
		t.Fatalf("row count = %d, want 1", rows.GetRowCount())
	}
	if value := singleIntValue(t, rows.GetRowBatches()); value != 37 {
		t.Fatalf("updated age = %d, want 37", value)
	}
}

func createSqlDbObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	// Create a SQL database object with an empty MySQL root.
	t.Helper()
	createdObject, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}

	// Register the created object as a SQL database.
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func createSqlSchemaObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	schema *s4wave_sql_schema.Schema,
) {
	// Create a World object containing the SQL schema root.
	t.Helper()
	createdObject, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(schema, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}

	// Register the schema type and synchronize its root relationships.
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
	_, sysErr, err := ws.ApplyWorldOp(ctx, s4wave_sql_schema_world.NewSqlSchemaSetRootOp(objectKey, rootRef), "")
	if err != nil || sysErr {
		t.Fatalf("SqlSchemaSetRootOp sysErr=%v err=%v", sysErr, err)
	}
}

func createSqlTableViewObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	tableView *s4wave_sql_table_view.TableView,
) {
	// Create a World object containing the table view root.
	t.Helper()
	createdObject, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(tableView, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}

	// Register the table view type and synchronize its root relationships.
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_table_view.SqlTableViewTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
	_, sysErr, err := ws.ApplyWorldOp(ctx, s4wave_sql_table_view_world.NewSqlTableViewSetRootOp(objectKey, rootRef), "")
	if err != nil || sysErr {
		t.Fatalf("SqlTableViewSetRootOp sysErr=%v err=%v", sysErr, err)
	}
}

func seedSqlDb(t *testing.T, ctx context.Context, tb *testbed.Testbed, objectKey string) {
	// Open the database resource used to seed the SQL fixture.
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

	// Create the alpha database through its SQL resource.
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSql(t, ctx, rootTx)

	// Populate the alpha database with the people table and its initial rows.
	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	for _, query := range []string{
		"CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL, age BIGINT NOT NULL)",
		"INSERT INTO people (id, name, age) VALUES (1, 'ada', 36)",
		"INSERT INTO people (id, name, age) VALUES (2, 'bob', 20)",
	} {
		execSql(t, ctx, writeTx, query)
	}
	commitSql(t, ctx, writeTx)
}

func openSqlSchemaClient(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	objectKey string,
) (s4wave_sql_schema.SRPCSqlSchemaResourceServiceClient, func()) {
	// Open the schema resource and connect its SRPC client.
	t.Helper()
	inv, cleanup, err := s4wave_sql_schema_world.SqlSchemaFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlSchemaFactory: %v", err)
	}
	client := s4wave_sql_schema.NewSRPCSqlSchemaResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv))),
	)
	return client, cleanup
}

func openSqlTableViewClient(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	objectKey string,
) (s4wave_sql_table_view.SRPCSqlTableViewResourceServiceClient, func()) {
	// Open the table view resource and connect its SRPC client.
	t.Helper()
	inv, cleanup, err := s4wave_sql_table_view_world.SqlTableViewFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.BusEngine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlTableViewFactory: %v", err)
	}
	client := s4wave_sql_table_view.NewSRPCSqlTableViewResourceServiceClient(
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

func hasTable(tables []*s4wave_sql_schema.TableInfo, tableName string) bool {
	for _, table := range tables {
		if table.GetName() == tableName {
			return true
		}
	}
	return false
}

func singleStringValue(t *testing.T, batches []*hydra_sql.RowBatch) string {
	// Require a single row batch for the string result.
	t.Helper()
	if len(batches) != 1 {
		t.Fatalf("row batches = %d, want 1", len(batches))
	}

	// Require a single row in the string result batch.
	rows := batches[0].GetRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}

	// Require a single column value in the string result row.
	values := rows[0].GetValues()
	if len(values) != 1 {
		t.Fatalf("values = %d, want 1", len(values))
	}

	// Decode the string result from its supported SQL value types.
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

func singleIntValue(t *testing.T, batches []*hydra_sql.RowBatch) int64 {
	// Require a single row batch for the integer result.
	t.Helper()
	if len(batches) != 1 {
		t.Fatalf("row batches = %d, want 1", len(batches))
	}

	// Require a single row in the integer result batch.
	rows := batches[0].GetRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}

	// Require a single column value in the integer result row.
	values := rows[0].GetValues()
	if len(values) != 1 {
		t.Fatalf("values = %d, want 1", len(values))
	}

	// Decode the integer result from its typed SQL value.
	value, ok := values[0].GetValue().(*hydra_sql.SqlValue_IntValue)
	if !ok {
		t.Fatalf("value = %#v, want int", values[0])
	}

	return value.IntValue
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
