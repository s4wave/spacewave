package mysql

import (
	"context"
	fmt "fmt"
	"io"
	"strings"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
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

	sq := NewMysql(oc, nil)
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
