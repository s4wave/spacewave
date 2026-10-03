//go:build !js

package main

import (
	"context"
	"database/sql"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/core"
	common "github.com/s4wave/spacewave/db/examples/common"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/sql/mysql/gormadapter"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func main() {
	// Configure the example context and debug logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the controller bus that resolves the example storage.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	// Enable verbose logging for the in-memory storage volume.
	verbose := true

	/*
		av, _, ref, err := common.AddStorageVolume(ctx, le, b, sr, verbose)
		if err != nil {
			panic(err)
		}
	*/
	// Resolve the in-memory volume and retain its controller directive.
	av, _, ref, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(
			&volume_kvtxinmem.Config{Verbose: verbose},
		),
		nil,
	)
	if err != nil {
		panic(err)
	}
	defer ref.Release()

	// Construct the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, ncRef, err := loader.WaitExecControllerRunning(ctx, b, dir, nil)
	if err != nil {
		panic(err)
	}
	defer ncRef.Release()
	le.Info("node controller resolved")

	// Access the storage volume exposed by its resolved controller.
	le.Info("storage volume resolved")
	volCtr := av.(volume.Controller)
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		panic(err)
	}

	// Create the bucket that stores the example MySQL database.
	bucketID := "test-bucket-mysql"
	volID := vol.GetID()
	_, _, _, err = vol.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  bucketID,
		Rev: 1,
	})
	if err != nil {
		panic(err)
	}

	// Open an empty bucket cursor for the MySQL storage engine.
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		b,
		le,
		nil,
		bucketID,
		volID,
		nil, nil,
	)
	if err != nil {
		panic(err)
	}

	// Run the go-orm demo.
	sq := mysql.NewMysql(oc, nil)
	dbName := "test-db"
	dsn := "/" + dbName
	buildTx := func(write bool) (*mysql.Tx, *gorm.DB, *sql.DB) {
		// Open the MySQL transaction used by the ORM adapter.
		tx, err := sq.NewMysqlTransaction(ctx, true)
		if err != nil {
			panic(err)
		}

		// assert that the database exists
		_, err = tx.OpenDatabase(ctx, dbName, true)
		if err != nil {
			panic(err)
		}

		// Attach the GORM adapter to the transaction and example database.
		db, sqlDB, err := gormadapter.NewMysqlGorm(
			ctx,
			le,
			tx,
			&gorm.Config{},
			dsn,
		)
		if err != nil {
			panic(err)
		}
		return tx, db, sqlDB
	}

	// Prepare the Entry table in a writable MySQL transaction.
	tx, db, _ := buildTx(true)
	if err := db.AutoMigrate(&Entry{}); err != nil {
		panic(err)
	}

	// Insert the three example entries through GORM.
	createVals := []*Entry{
		{Value: 4, ID: 1},
		{Value: 10, ID: 2},
		{Value: 30, ID: 3},
	}
	for _, v := range createVals {
		db.Create(v)
	}

	// Commit the example entries to the bucket-backed database.
	err = tx.Commit(ctx)
	if err != nil {
		panic(err)
	}
	le.Infof("successfully stored %d objects", 3)

	// Read all persisted entries and require the complete result set.
	tx, db, _ = buildTx(false)
	_ = tx
	var se []Entry
	out := db.Find(&se)
	if out.Error != nil {
		panic(out.Error)
	}
	if len(se) != 3 {
		panic(errors.Errorf("expected 3 results but got %d", len(se)))
	}
	le.Infof("successfully retrieved %d objects", len(se))

	// Verify that a GORM value query retrieves the matching entry.
	var e Entry
	out = db.Where("value = ?", 30).Find(&e)
	if out.Error != nil {
		panic(out.Error)
	}
	if e.Value != 30 {
		panic("value was incorrect")
	}
	le.Infof("successfully retrieved object by value lookup: %#v", e)

	// Release the transaction after the example queries finish.
	tx.Discard()
}

// Entry is an entry in the database.
type Entry struct {
	ID    int `gorm:"primaryKey"`
	Value int `json:"value"`
}

var _ any = common.AddStorageVolume
