package mysql_controller

import (
	"context"
	"testing"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql_mock "github.com/s4wave/spacewave/db/sql/mock"
	mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestMysqlDb performs a simple test of operations against the db.
func TestMysqlDb(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	sfs := transform_all.BuildFactorySet()

	dbID := "test-db"
	bucketID := dbID
	objStoreID := dbID

	bucketConf, err := bucket.NewConfig(bucketID, 1, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = bucket.ExApplyBucketConfig(
		ctx,
		tb.Bus,
		bucket.NewApplyBucketConfig(
			bucketConf,
			nil,
			[]string{tb.Volume.GetID()},
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	dbName := "test-db"
	conf := &Config{
		SqlDbId:       dbID,
		BucketId:      bucketID,
		VolumeId:      tb.Volume.GetID(),
		ObjectStoreId: objStoreID,
		CreateDbs:     []string{dbName},
	}

	ctrl, err := NewController(le, tb.Bus, conf, sfs)
	if err != nil {
		t.Fatal(err.Error())
	}

	relCtrl, err := tb.Bus.AddController(ctx, ctrl, func(err error) {
		if err != nil && err != context.Canceled {
			t.Fatal(err.Error())
		}
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relCtrl()

	// init data
	tableName := "test-table"
	rctx := sql.NewEmptyContext().WithContext(ctx)
	rctx.SetCurrentDatabase(dbName)
	sdb, err := ctrl.GetSqlStore(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	msql := sdb.(*mysql.Mysql)
	tx, err := msql.NewMysqlTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// create=false because we are testing CreateDbs above
	db, err := tx.OpenDatabase(ctx, dbName, false)
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
	err = db.CreateTable(rctx, tableName, pkSchema, sql.Collation_Default, "testing table")
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

	// tests
	err = hydra_sql_mock.TestSqlStore_Basic(ctx, le, sdb, "/"+dbName)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}
