//go:build !sql_lite

package s4wave_sql_world_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
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
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/test-db"
	beforeRoot := createSqlDbObject(t, ctx, tb.WorldState, objectKey, true)

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

	readTx := openSqlTx(t, ctx, store, false, "/alpha")
	name := querySingleString(t, ctx, readTx, "SELECT name FROM people WHERE id = 1")
	readTx.Discard()
	if name != "ada" {
		t.Fatalf("SELECT name = %q, want ada", name)
	}

	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root did not advance")
	}

	lookupOp, err := optypes.LookupWorldOp(ctx, s4wave_sql_world.SqlSetRootOpId)
	if err != nil {
		t.Fatalf("LookupWorldOp(%s): %v", s4wave_sql_world.SqlSetRootOpId, err)
	}
	if _, ok := lookupOp.(*s4wave_sql_world.SqlSetRootOp); !ok {
		t.Fatalf("LookupWorldOp returned %T, want *SqlSetRootOp", lookupOp)
	}

	op := s4wave_sql_world.NewSqlSetRootOp(objectKey, afterRoot, afterRoot, nil)
	data, err := op.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	if err := lookupOp.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}
	if !lookupOp.(*s4wave_sql_world.SqlSetRootOp).GetRootRef().EqualsRef(afterRoot) {
		t.Fatal("replayed SqlSetRootOp root ref did not round-trip")
	}
	if _, sysErr, err := tb.WorldState.ApplyWorldOp(ctx, lookupOp, ""); err != nil || sysErr {
		t.Fatalf("replay ApplyWorldOp sysErr=%v err=%v", sysErr, err)
	}
}

func TestWorldBackedSqlReportsCommitPersistedWhenWorldRootUpdateFails(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "sql/untyped-db"
	beforeRoot := createSqlDbObject(t, ctx, tb.WorldState, objectKey, false)
	store, cleanup := openWorldBackedSql(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	rootTx := openSqlTx(t, ctx, store, true, "")
	execSql(t, ctx, rootTx, "CREATE DATABASE alpha")
	commitSqlPersisted(t, ctx, rootTx)

	writeTx := openSqlTx(t, ctx, store, true, "/alpha")
	for _, query := range []string{
		"CREATE TABLE persisted (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO persisted (id, name) VALUES (1, 'inner-root')",
	} {
		execSql(t, ctx, writeTx, query)
	}
	commitSqlPersisted(t, ctx, writeTx)

	readTx := openSqlTx(t, ctx, store, false, "/alpha")
	name := querySingleString(t, ctx, readTx, "SELECT name FROM persisted WHERE id = 1")
	readTx.Discard()
	if name != "inner-root" {
		t.Fatalf("inner SELECT name = %q, want inner-root", name)
	}

	afterRoot := getObjectRoot(t, ctx, tb.WorldState, objectKey)
	if !beforeRoot.EqualsRef(afterRoot) {
		t.Fatal("world object root advanced despite failed ApplyWorldOp")
	}
}

func createSqlDbObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	setType bool,
) *bucket.ObjectRef {
	t.Helper()
	_, rootRef, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(sql_mysql.NewRootBlock(), true)
		return nil
	})
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
	if _, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		return nil
	}); err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_sql_world.SqlDbTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func getObjectRoot(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) *bucket.ObjectRef {
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
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
	t.Helper()
	ops, err := tx.GetSqlOps(ctx)
	if err != nil {
		tx.Discard()
		t.Fatalf("GetSqlOps: %v", err)
	}
	rows, err := ops.QueryContext(ctx, query, nil)
	if err != nil {
		tx.Discard()
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	cols := rows.Columns()
	if len(cols) != 1 {
		tx.Discard()
		t.Fatalf("%s columns = %v, want one column", query, cols)
	}
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		tx.Discard()
		t.Fatalf("%s next: %v", query, err)
	}
	if err := rows.Next(dest); err != io.EOF {
		tx.Discard()
		t.Fatalf("%s next after row = %v, want EOF", query, err)
	}
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
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}
	var store *s4wave_sql_world.WorldBackedSql
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = s4wave_sql_world.NewWorldBackedSql(ctx, root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		t.Fatalf("AccessWorldState(%s): %v", objectKey, err)
	}
	return store, store.Close
}
