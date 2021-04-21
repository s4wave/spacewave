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
	"github.com/sirupsen/logrus"
)

// TODO enginetest from go-mysql-server

// TestMysql runs the sql engine test suite.
func TestMysql(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testbed.Verbose = true
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

	sq := NewMysql(oc)
	tx, err := sq.NewMysqlTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tableName := "test-table"
	dbName := "test-db"
	rctx := sql.NewEmptyContext().WithContext(ctx).WithCurrentDB(dbName)
	db, err := tx.OpenDatabase(dbName, true)
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
	err = db.CreateTable(rctx, tableName, sql.Schema{
		{Name: "name", Type: sql.Text, Nullable: false, Source: tableName},
		{Name: "email", Type: sql.Text, Nullable: false, Source: tableName},
		{Name: "phone_numbers", Type: sql.JSON, Nullable: false, Source: tableName},
		{Name: "created_at", Type: sql.Timestamp, Nullable: false, Source: tableName},
	})
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

	tx, err = sq.NewMysqlTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	db, err = tx.OpenDatabase(dbName, false)
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
		tx, err := sq.NewMysqlTransaction(true)
		if err != nil {
			t.Fatal(err.Error())
		}
		db, err = tx.OpenDatabase(dbName, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		e := sqle.NewDefault()
		e.AddDatabase(db)
		return tx, e
	}
	buildSqlCtx := func() *sql.Context {
		ssess := sql.NewSession("address", "client", "user", 1)
		sqlCtx := sql.NewContext(ctx,
			sql.WithSession(ssess),
			sql.WithIndexRegistry(sql.NewIndexRegistry()),
			sql.WithViewRegistry(sql.NewViewRegistry()),
		)
		_ = sqlCtx.Set(sqlCtx, sql.AutoCommitSessionVar, sql.Boolean, true)
		sqlCtx.SetCurrentDatabase(dbName)
		return sqlCtx
	}

	printQuery := func(e *sqle.Engine, query string) int {
		sqlCtx := buildSqlCtx()
		t.Logf("QUERY: %s", query)
		_, r, err := e.Query(sqlCtx, query)
		if err != nil {
			t.Fatal(err.Error())
		}
		var nrows int
		for {
			row, err := r.Next()
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
	printQuery(e,
		fmt.Sprintf(
			"INSERT INTO `%s` (name, email, created_at, phone_numbers) VALUES ('name', 'my@email.com', NOW(), '[\"555-555-5555\"]')",
			tableName,
		),
	)

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	tx, e = buildEngine()
	printQuery(e, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	tx.Discard()
}
