package s4wave_sql_world_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestSqlDbFactoryCommitsWorldBackedRootAndReplaysOp(t *testing.T) {
	// Start the testbed for SQL root persistence and operation replay.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create a typed SQL World object and retain its initial root.
	objectKey := "sql/test-db"
	beforeRoot := createSqlDbObject(t, ctx, tb.WorldState, objectKey, true)

	// Open the SQL World object through an SRPC client.
	inv, cleanup, err := s4wave_sql_world.SqlDbFactory(
		ctx,
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		tb.Engine,
		tb.WorldState,
		objectKey,
	)
	if err != nil {
		t.Fatalf("SqlDbFactory: %v", err)
	}
	defer cleanup()

	// Commit the alpha database through the SRPC SQL store.
	store := sql_rpc_client.NewStore(sql_rpc.NewSRPCSqlClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(inv)))))
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSql(t, ctx, rootTx)

	// Commit the people table and its first row.
	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	for _, query := range []string{
		"CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO people (id, name) VALUES (1, 'ada')",
	} {
		execSql(t, ctx, writeTx, query)
	}
	commitSql(t, ctx, writeTx)

	// Read the committed people row and verify its value.
	readTx := openSqlTx(t, ctx, store, false, "/alpha")
	name := querySingleString(t, ctx, readTx, "SELECT name FROM people WHERE id = 1")
	readTx.Discard()
	if name != "ada" {
		t.Fatalf("SELECT name = %q, want ada", name)
	}

	// Verify that the SQL commit advanced the World object root.
	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root did not advance")
	}

	// Resolve the SQL root operation by its registered identifier.
	lookupOp, err := world.NewSchema(s4wave_sql_world.WorldTypes).LookupOp(ctx, s4wave_sql_world.SqlSetRootOpTypeID)
	if err != nil {
		t.Fatalf("LookupSqlSetRootOp(%s): %v", s4wave_sql_world.SqlSetRootOpTypeID, err)
	}
	if _, ok := lookupOp.(*s4wave_sql_world.SqlSetRootOp); !ok {
		t.Fatalf("LookupSqlSetRootOp returned %T, want *SqlSetRootOp", lookupOp)
	}

	// Round-trip the SQL root operation and replay it against the World object.
	op := s4wave_sql_world.NewSqlSetRootOp(objectKey, afterRoot, afterRoot, nil)
	data, err := op.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	if err := lookupOp.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !lookupOp.(*s4wave_sql_world.SqlSetRootOp).GetRootRef().EqualsRef(afterRoot) {
		t.Fatal("replayed SqlSetRootOp root ref did not round-trip")
	}
	if _, sysErr, err := tb.WorldState.ApplyWorldOp(ctx, lookupOp, ""); err != nil || sysErr {
		t.Fatalf("replay ApplyWorldOp sysErr=%v err=%v", sysErr, err)
	}
}

func TestWorldBackedSqlReportsCommitPersistedWhenWorldRootUpdateFails(t *testing.T) {
	// Start the testbed for SQL commit persistence failures.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open an untyped SQL World object whose root update will fail.
	objectKey := "sql/untyped-db"
	beforeRoot := createSqlDbObject(t, ctx, tb.WorldState, objectKey, false)
	store, cleanup := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Commit the database and require the persisted-commit error.
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSqlPersisted(t, ctx, rootTx)

	// Block both readers and writers from observing an unpublished private root.
	for _, write := range []bool{false, true} {
		if tx, err := store.NewSqlTransaction(ctx, write, "/alpha"); !errors.Is(err, s4wave_sql_world.ErrCommitPersisted) {
			if tx != nil {
				tx.Discard()
			}
			t.Fatalf("transaction after publication failure: write=%v err=%v", write, err)
		}
	}

	// Verify the rejected World operation left the object root unchanged.
	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if !beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root advanced despite failed ApplyWorldOp")
	}
}

func TestWorldBackedSqlUpsertAndReplace(t *testing.T) {
	// Start the testbed and open a typed SQL World object.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	objectKey := "sql/upsert-db"
	createSqlDbObject(t, ctx, tb.WorldState, objectKey, true)
	store, cleanup := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Create the database and a table with a primary and a unique key.
	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE shop")
	commitSql(t, ctx, rootTx)
	writeTx := openSqlTx(t, ctx, store, true, "/shop")
	for _, query := range []string{
		"CREATE TABLE items (id BIGINT NOT NULL PRIMARY KEY, sku VARCHAR(32) NOT NULL UNIQUE, qty BIGINT NOT NULL)",
		"INSERT INTO items (id, sku, qty) VALUES (1, 'a', 1), (2, 'b', 2)",
		// A primary key conflict updates the existing row in each form.
		"INSERT INTO items (id, sku, qty) VALUES (1, 'a', 9) ON DUPLICATE KEY UPDATE qty = 5",
		"INSERT INTO items (id, sku, qty) VALUES (1, 'a', 3) ON DUPLICATE KEY UPDATE qty = qty + VALUES(qty)",
		"INSERT INTO items (id, sku, qty) VALUES (2, 'b', 7) AS new ON DUPLICATE KEY UPDATE qty = new.qty",
		// A unique key conflict updates the row holding that key.
		"INSERT INTO items (id, sku, qty) VALUES (3, 'b', 4) ON DUPLICATE KEY UPDATE qty = qty + 1",
		// REPLACE deletes the conflicting row and inserts the new one.
		"REPLACE INTO items (id, sku, qty) VALUES (1, 'z', 6)",
		"REPLACE INTO items (id, sku, qty) VALUES (4, 'b', 10)",
	} {
		execSql(t, ctx, writeTx, query)
	}
	commitSql(t, ctx, writeTx)

	// Verify the committed rows after the upserts and replacements.
	readTx := openSqlTx(t, ctx, store, false, "/shop")
	defer readTx.Discard()
	rows := querySingleString(t, ctx, readTx, "SELECT GROUP_CONCAT(CONCAT(id, ':', sku, ':', qty) ORDER BY id) FROM items")
	if want := "1:z:6,4:b:10"; rows != want {
		t.Fatalf("items = %q, want %q", rows, want)
	}
}

func createSqlDbObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	setType bool,
) *bucket.ObjectRef {
	// Create a SQL World object with an initial database root and optional type.
	t.Helper()
	createdObject, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if setType {
		if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
			t.Fatalf("SetObjectType(%s): %v", objectKey, err)
		}
	}
	return rootRef.Clone()
}

// createEmptySqlDbObject creates a sql/db object with an empty initial root,
// mirroring the browser quickstart path that creates the object before any root
// block is written, so the first transaction opens against an empty root.
func createEmptySqlDbObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	{
		createdObject, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
		}
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func getObjectRoot(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) *bucket.ObjectRef {
	// Resolve the World object and read its root reference.
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}
	root, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatalf("GetRootRef(%s): %v", objectKey, err)
	}
	return root.Clone()
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

func commitSqlPersisted(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction) {
	t.Helper()
	if err := tx.Commit(ctx); !errors.Is(err, s4wave_sql_world.ErrCommitPersisted) {
		tx.Discard()
		t.Fatalf("Commit error = %v, want ErrCommitPersisted", err)
	}
}

func querySingleString(t *testing.T, ctx context.Context, tx hydra_sql.SqlTransaction, query string) string {
	// Obtain SQL operations and query the expected single string value.
	t.Helper()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		tx.Discard()
		t.Fatalf("GetSqlOps: %v", err)
	}

	// Run the SQL query and release its row cursor on exit.
	rows, err := ops.QueryContext(ctx, query, nil)
	if err != nil {
		tx.Discard()
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()

	// Require the SQL query to return exactly one column.
	cols := rows.Columns()
	if len(cols) != 1 {
		tx.Discard()
		t.Fatalf("%s columns = %v, want one column", query, cols)
	}

	// Read the single SQL row and require the cursor to end afterward.
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		tx.Discard()
		t.Fatalf("%s next: %v", query, err)
	}
	if err := rows.Next(dest); err != io.EOF {
		tx.Discard()
		t.Fatalf("%s next after row = %v, want EOF", query, err)
	}

	// Decode the SQL value as a string for the caller.
	switch val := dest[0].(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		tx.Discard()
		t.Fatalf("%s value = %#v, want string", query, dest[0])
	}
	return ""
}

func openWorldBackedSql(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (hydra_sql.SqlStore, func()) {
	// Resolve the World object and open its SQL store through a storage cursor.
	t.Helper()
	store, err := s4wave_sql_world.NewWorldBackedSql(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("NewWorldBackedSql(%s): %v", objectKey, err)
	}
	return store, store.Close
}
