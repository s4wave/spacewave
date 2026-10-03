package mysql_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	hydra_sql_mock "github.com/s4wave/spacewave/db/sql/mock"
	mysql "github.com/s4wave/spacewave/db/sql/mysql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_client "github.com/s4wave/spacewave/db/sql/rpc/client"
	"github.com/s4wave/spacewave/db/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestMysqlDb performs a simple test of operations against the db.
func TestMysqlDb(t *testing.T) {
	// Prepare the logger and context for the SQL controller test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Open a storage testbed for the SQL database and RPC service.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Acquire an empty bucket cursor for the storage fixture.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Prepare the SQL database identifiers and block transform factories.
	sfs := transform_all.BuildFactorySet()
	dbID := "test-db"
	bucketID := dbID
	objStoreID := dbID

	// Configure the SQL bucket on the testbed volume.
	bucketConf, err := bucket.NewConfig(bucketID, 1, nil)
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

	// Configure the controller to create a database and publish its RPC service.
	dbName := "test-db"
	sqlRpcServiceID := "test/sql/rpc"
	conf := &Config{
		SqlDbId:         dbID,
		BucketId:        bucketID,
		VolumeId:        tb.Volume.GetID(),
		ObjectStoreId:   objStoreID,
		CreateDbs:       []string{dbName},
		SqlRpcServiceId: sqlRpcServiceID,
	}

	// Construct the MySQL controller with the storage configuration.
	ctrl, err := NewController(le, tb.Bus, conf, sfs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Attach the MySQL controller for the duration of the test.
	relCtrl, err := tb.Bus.AddController(ctx, ctrl, func(err error) {
		if err != nil && err != context.Canceled {
			t.Fatal(err.Error())
		}
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relCtrl()

	// Open the SQL store and a write transaction for the initial table.
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

	// Open the database created by the controller configuration.
	// create=false because we are testing CreateDbs above
	db, err := tx.OpenDatabase(ctx, dbName, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the configured database begins without tables.
	names, err := db.GetTableNames(rctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(names) != 0 {
		t.Fatal("expected db to start empty")
	}

	// Create the test table with its primary key and typed columns.
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

	// Verify the database exposes the newly created table.
	names, err = db.GetTableNames(rctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(names) != 1 || names[0] != tableName {
		t.Fatalf("unexpected table names: %v", names)
	}

	// Commit the initial table before exercising SQL store operations.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// tests
	err = hydra_sql_mock.TestSqlStore_Basic(ctx, le, sdb, "/"+dbName)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the SQL controller RPC service and retain its reference.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(ctx, tb.Bus, sqlRpcServiceID, "", true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(invokers) != 1 {
		t.Fatalf("expected one sql rpc invoker, got %d", len(invokers))
	}
	defer invokerRef.Release()

	// Exercise the SQL store contract through the published RPC service.
	rpcClient := sql_rpc.NewSRPCSqlClientWithServiceID(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))),
		sqlRpcServiceID,
	)
	rpcStore := sql_rpc_client.NewStore(rpcClient)
	err = hydra_sql_mock.TestSqlStore_Basic(ctx, le, rpcStore, "/"+dbName)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}
