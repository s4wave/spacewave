package mysql

import (
	"context"
	stdsql "database/sql"
	fmt "fmt"
	"io"
	"strings"
	"testing"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

var verbose = false

// TODO enginetest from go-mysql-server

// TestMysql runs the sql engine test suite.
func TestMysql(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = verbose
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	var committedRoot string
	sq := NewMysql(oc, func(ref *bucket.ObjectRef) error {
		committedRoot = ref.MarshalString()
		return nil
	})
	tx, err := sq.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tableName := "test-table"
	dbName := "test-db"
	rctx := sql.NewEmptyContext().WithContext(ctx)
	rctx.SetCurrentDatabase(dbName)
	db, err := tx.OpenDatabase(ctx, dbName, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	names, err := db.GetTableNames(rctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(names) != 0 {
		t.Fatal("expected db to start empty")
	}
	pkSchema := sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: types.Int64, Nullable: false, Source: tableName, PrimaryKey: true, AutoIncrement: true},
		{Name: "name", Type: types.Text, Nullable: false, Source: tableName},
		{Name: "email", Type: types.Text, Nullable: false, Source: tableName},
		{Name: "phone_numbers", Type: types.JSON, Nullable: false, Source: tableName},
		{Name: "created_at", Type: types.Timestamp, Nullable: false, Source: tableName},
	})
	err = db.CreateTable(rctx, tableName, pkSchema, sql.Collation_Default, "demo table")
	if err != nil {
		t.Fatal(err.Error())
	}
	names, err = db.GetTableNames(rctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(names) != 1 || names[0] != tableName {
		t.Fatalf("unexpected table names: %v", names)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if committedRoot == "" {
		t.Fatal("expected committed root")
	}

	tx, err = sq.NewMysqlTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	db, err = tx.OpenDatabase(ctx, dbName, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	names, err = db.GetTableNames(rctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(names) != 1 || names[0] != tableName {
		t.Fatalf("unexpected table names: %v", names)
	}

	tbl, ok, err := db.GetTableInsensitive(rctx, strings.ToUpper(tableName))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected case insensitive table lookup to return result")
	}
	if tn := tbl.Name(); tn != tableName {
		t.Fatalf("expected %s got %s", tableName, tn)
	}
	tx.Discard()

	buildEngine := func() (*Tx, *sqle.Engine) {
		tx, err := sq.NewMysqlTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		db, err = tx.OpenDatabase(ctx, dbName, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		prov, err := tx.BuildDatabaseProvider(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		e := sqle.NewDefault(prov)
		return tx, e
	}
	buildSqlCtx := func() *sql.Context {
		sclient := sql.Client{
			User:    "hydra",
			Address: "inproc",
		}
		ssess := sql.NewBaseSessionWithClientServer("address", sclient, 1)
		sqlCtx := sql.NewContext(ctx,
			sql.WithSession(ssess),
			// sql.WithIndexRegistry(sql.NewIndexRegistry()),
			// sql.WithViewRegistry(sql.NewViewRegistry()),
		)
		_ = sqlCtx.SetUserVariable(sqlCtx, sql.AutoCommitSessionVar, true, types.Boolean)
		sqlCtx.SetCurrentDatabase(dbName)
		return sqlCtx
	}

	printQuery := func(e *sqle.Engine, query string) int {
		sqlCtx := buildSqlCtx()
		t.Logf("QUERY: %s", query)
		_, r, _, err := e.Query(sqlCtx, query)
		if err != nil {
			t.Fatal(err.Error())
		}
		var nrows int
		for {
			row, err := r.Next(sqlCtx)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err.Error())
			}
			nrows++
			for ci, col := range row {
				t.Logf("COL %d: %v", ci, col)
			}
		}
		t.Logf("END QUERY: %d rows", nrows)
		r.Close(sqlCtx)
		return nrows
	}

	// test sql queries
	tx, e := buildEngine()

	printQuery(e, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	for i := range 3 {
		printQuery(e,
			fmt.Sprintf(
				"INSERT INTO `%s` (name, email, created_at, phone_numbers) VALUES ('entry-%d', 'account-%d@email.com', NOW(), '[\"555-555-555%d\"]')",
				tableName,
				i, i, i,
			),
		)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	tx, e = buildEngine()
	printQuery(e, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	tx.Discard()
}

func TestMysqlReadInsertedRowBeforeCommit(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	tx, bcs := block.NewTransaction(store, nil, nil, nil)
	bcs.SetBlock(NewDatabaseRootBlock(), true)
	db, err := NewDatabase(ctx, "r2sql", false, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	sqlCtx := sql.NewContext(ctx, sql.WithSession(sql.NewBaseSession()))
	sqlCtx.SetCurrentDatabase("r2sql")
	err = db.CreateTable(sqlCtx, "kv", sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: types.Int64, Nullable: false, Source: "kv", PrimaryKey: true},
		{Name: "name", Type: types.Text, Nullable: false, Source: "kv"},
		{Name: "bytes", Type: types.Int64, Nullable: false, Source: "kv"},
	}), sql.Collation_Default, "")
	if err != nil {
		t.Fatal(err.Error())
	}
	tbl, ok, err := db.GetTableInsensitive(sqlCtx, "kv")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected table")
	}
	editor := tbl.(sql.InsertableTable).Inserter(sqlCtx)
	if err := editor.Insert(sqlCtx, sql.NewRow(int64(1), "alpha", int64(11))); err != nil {
		t.Fatal(err.Error())
	}
	if err := editor.Close(sqlCtx); err != nil {
		t.Fatal(err.Error())
	}
	partIter, err := tbl.Partitions(sqlCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	part, err := partIter.Next(sqlCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	rowIter, err := tbl.PartitionRows(sqlCtx, part)
	if err != nil {
		t.Fatal(err.Error())
	}
	row, err := rowIter.Next(sqlCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(row) != 3 || row[0] != int64(1) || row[1] != "alpha" || row[2] != int64(11) {
		t.Fatalf("unexpected row: %#v", row)
	}
	db.MarkDirty()
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if root.GetEmpty() {
		t.Fatal("expected database root")
	}
}

func TestMysqlUpdateSingleTableWhere(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	sq := NewMysql(oc, nil)
	tx, err := sq.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tx.OpenDatabase(ctx, "probe", true); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	db := hydra_sql.NewSqlDb(sq, "/probe")
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, query := range []string{
		"CREATE TABLE update_items (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL, qty BIGINT NOT NULL)",
		"INSERT INTO update_items (id, name, qty) VALUES (1, 'alpha', 7)",
		"INSERT INTO update_items (id, name, qty) VALUES (2, 'beta', 3)",
		"UPDATE update_items SET qty = 11, name = 'alpha-updated' WHERE id = 1",
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	var name string
	var qty int64
	if err := db.QueryRowContext(ctx, "SELECT name, qty FROM update_items WHERE id = 1").Scan(&name, &qty); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha-updated" || qty != 11 {
		t.Fatalf("unexpected updated row: name=%q qty=%d", name, qty)
	}

	if err := db.QueryRowContext(ctx, "SELECT name, qty FROM update_items WHERE id = 2").Scan(&name, &qty); err != nil {
		t.Fatal(err.Error())
	}
	if name != "beta" || qty != 3 {
		t.Fatalf("unexpected untouched row: name=%q qty=%d", name, qty)
	}

	db.Close()
	db = hydra_sql.NewSqlDb(sq, "/probe")
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := db.QueryRowContext(ctx, "SELECT name, qty FROM update_items WHERE id = 1").Scan(&name, &qty); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha-updated" || qty != 11 {
		t.Fatalf("unexpected reopened row: name=%q qty=%d", name, qty)
	}
}

func TestMysqlDeleteSingleTableWhere(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE delete_items (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO delete_items (id, name) VALUES (1, 'alpha')",
		"INSERT INTO delete_items (id, name) VALUES (2, 'beta')",
		"DELETE FROM delete_items WHERE id = 1",
	)
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM delete_items").Scan(&count); err != nil {
		t.Fatal(err.Error())
	}
	if count != 1 {
		t.Fatalf("unexpected row count after delete: %d", count)
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM delete_items WHERE id = 2").Scan(&name); err != nil {
		t.Fatal(err.Error())
	}
	if name != "beta" {
		t.Fatalf("unexpected remaining row: %q", name)
	}
}

func TestMysqlAlterAddDropColumn(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE alter_items (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO alter_items (id, name) VALUES (1, 'alpha')",
		"ALTER TABLE alter_items ADD COLUMN note TEXT NULL",
		"UPDATE alter_items SET note = 'ready' WHERE id = 1",
	)
	var name, note string
	if err := db.QueryRowContext(ctx, "SELECT name, note FROM alter_items WHERE id = 1").Scan(&name, &note); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha" || note != "ready" {
		t.Fatalf("unexpected altered row: name=%q note=%q", name, note)
	}
	execMysqlStatements(t, ctx, db, "ALTER TABLE alter_items DROP COLUMN note")
	if err := db.QueryRowContext(ctx, "SELECT name FROM alter_items WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha" {
		t.Fatalf("unexpected row after drop column: %q", name)
	}
	if _, err := db.QueryContext(ctx, "SELECT note FROM alter_items"); err == nil {
		t.Fatal("expected dropped column query to fail")
	}
}

func TestMysqlDropDatabaseAndDropTable(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE DATABASE doomed",
		"USE doomed",
		"CREATE TABLE drop_items (id BIGINT NOT NULL PRIMARY KEY)",
		"INSERT INTO drop_items (id) VALUES (1)",
		"DROP TABLE drop_items",
	)
	if _, err := db.QueryContext(ctx, "SELECT * FROM drop_items"); err == nil {
		t.Fatal("expected dropped table query to fail")
	}
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE kept_until_db_drop (id BIGINT NOT NULL PRIMARY KEY)",
		"USE probe",
		"DROP DATABASE doomed",
	)
	if _, err := db.ExecContext(ctx, "USE doomed"); err == nil {
		t.Fatal("expected dropped database USE to fail")
	}
}

func TestMysqlPrimaryKeyEnforcedAndIndexedLookup(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE pk_items (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO pk_items (id, name) VALUES (1, 'alpha')",
		"INSERT INTO pk_items (id, name) VALUES (2, 'beta')",
	)
	if _, err := db.ExecContext(ctx, "INSERT INTO pk_items (id, name) VALUES (1, 'dupe')"); err == nil {
		t.Fatal("expected duplicate primary key insert to fail")
	}
	if _, err := db.ExecContext(ctx, "UPDATE pk_items SET id = 1 WHERE id = 2"); err == nil {
		t.Fatal("expected duplicate primary key update to fail")
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM pk_items WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha" {
		t.Fatalf("unexpected indexed lookup row: %q", name)
	}
}

func TestMysqlSecondaryCreateDropIndex(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE index_items (id BIGINT NOT NULL PRIMARY KEY, qty BIGINT NOT NULL)",
		"INSERT INTO index_items (id, qty) VALUES (1, 7)",
		"INSERT INTO index_items (id, qty) VALUES (2, 9)",
		"CREATE INDEX idx_qty ON index_items (qty)",
	)
	if _, err := db.ExecContext(ctx, "CREATE INDEX idx_qty ON index_items (qty)"); err == nil {
		t.Fatal("expected duplicate index create to fail")
	}
	var id int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM index_items WHERE qty = 7").Scan(&id); err != nil {
		t.Fatal(err.Error())
	}
	if id != 1 {
		t.Fatalf("unexpected indexed query id: %d", id)
	}
	execMysqlStatements(t, ctx, db, "DROP INDEX idx_qty ON index_items")
	if _, err := db.ExecContext(ctx, "DROP INDEX idx_qty ON index_items"); err == nil {
		t.Fatal("expected duplicate index drop to fail")
	}
}

func TestMysqlJoinRegression(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE users (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"CREATE TABLE orders (id BIGINT NOT NULL PRIMARY KEY, user_id BIGINT NOT NULL, item TEXT NOT NULL)",
		"INSERT INTO users (id, name) VALUES (1, 'alpha')",
		"INSERT INTO users (id, name) VALUES (2, 'beta')",
		"INSERT INTO orders (id, user_id, item) VALUES (10, 1, 'book')",
	)
	var name, item string
	if err := db.QueryRowContext(ctx, "SELECT users.name, orders.item FROM users INNER JOIN orders ON users.id = orders.user_id").Scan(&name, &item); err != nil {
		t.Fatal(err.Error())
	}
	if name != "alpha" || item != "book" {
		t.Fatalf("unexpected inner join row: name=%q item=%q", name, item)
	}
	rows, err := db.QueryContext(ctx, "SELECT users.name, orders.item FROM users LEFT JOIN orders ON users.id = orders.user_id ORDER BY users.id")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rows.Close()
	var seen int
	for rows.Next() {
		var rowName string
		var rowItem stdsql.NullString
		if err := rows.Scan(&rowName, &rowItem); err != nil {
			t.Fatal(err.Error())
		}
		seen++
		if rowName == "beta" && rowItem.Valid {
			t.Fatalf("expected beta left join item to be NULL, got %q", rowItem.String)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err.Error())
	}
	if seen != 2 {
		t.Fatalf("unexpected left join row count: %d", seen)
	}
}

func TestMysqlDiscardChangesRollsBackFailedStatement(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE TABLE rollback_items (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"INSERT INTO rollback_items (id, name) VALUES (1, 'one')",
	)
	if _, err := db.ExecContext(ctx, "INSERT INTO rollback_items (id, name) VALUES (2, 'two'), (1, 'dupe')"); err == nil {
		t.Fatal("expected duplicate primary key statement to fail")
	}
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rollback_items").Scan(&count); err != nil {
		t.Fatal(err.Error())
	}
	if count != 1 {
		t.Fatalf("expected failed statement rollback to keep one row, got %d", count)
	}
}

func TestMysqlCrudUnsupportedOpsRegressionSweep(t *testing.T) {
	ctx := context.Background()
	_, db := newMysqlTestDB(t, ctx, "probe")
	defer db.Close()
	execMysqlStatements(t, ctx, db,
		"CREATE DATABASE sweep_db",
		"DROP DATABASE sweep_db",
		"CREATE TABLE sweep_left (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL)",
		"CREATE TABLE sweep_right (id BIGINT NOT NULL PRIMARY KEY, left_id BIGINT NOT NULL, note TEXT NOT NULL)",
		"INSERT INTO sweep_left (id, name) VALUES (1, 'one')",
		"INSERT INTO sweep_left (id, name) VALUES (2, 'two')",
		"INSERT INTO sweep_right (id, left_id, note) VALUES (10, 1, 'joined')",
		"UPDATE sweep_left SET name = 'one-updated' WHERE id = 1",
		"DELETE FROM sweep_left WHERE id = 2",
		"ALTER TABLE sweep_left ADD COLUMN marker BIGINT NULL",
		"UPDATE sweep_left SET marker = 42 WHERE id = 1",
		"CREATE INDEX sweep_marker_idx ON sweep_left (marker)",
		"DROP INDEX sweep_marker_idx ON sweep_left",
	)
	var name, note string
	var marker int64
	if err := db.QueryRowContext(ctx, "SELECT sweep_left.name, sweep_left.marker, sweep_right.note FROM sweep_left INNER JOIN sweep_right ON sweep_left.id = sweep_right.left_id").Scan(&name, &marker, &note); err != nil {
		t.Fatal(err.Error())
	}
	if name != "one-updated" || marker != 42 || note != "joined" {
		t.Fatalf("unexpected sweep inner join row: name=%q marker=%d note=%q", name, marker, note)
	}
	execMysqlStatements(t, ctx, db,
		"ALTER TABLE sweep_left DROP COLUMN marker",
		"CREATE TABLE z_sweep_drop (id BIGINT NOT NULL PRIMARY KEY)",
		"DROP TABLE z_sweep_drop",
	)
	if _, err := db.ExecContext(ctx, "INSERT INTO sweep_left (id, name) VALUES (1, 'duplicate')"); err == nil {
		t.Fatal("expected sweep duplicate primary key to fail")
	}
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sweep_left LEFT JOIN sweep_right ON sweep_left.id = sweep_right.left_id").Scan(&count); err != nil {
		t.Fatal(err.Error())
	}
	if count != 1 {
		t.Fatalf("unexpected sweep left join count: %d", count)
	}
}

func newMysqlTestDB(t *testing.T, ctx context.Context, dbName string) (*Mysql, *stdsql.DB) {
	t.Helper()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	sq := NewMysql(oc, nil)
	tx, err := sq.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tx.OpenDatabase(ctx, dbName, true); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	db := hydra_sql.NewSqlDb(sq, "/"+dbName)
	db.SetMaxOpenConns(1)
	return sq, db
}

func execMysqlStatements(t *testing.T, ctx context.Context, db *stdsql.DB, queries ...string) {
	t.Helper()
	for _, query := range queries {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
}
