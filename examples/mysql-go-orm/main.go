package main

import (
	"context"
	"database/sql"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/sql/mysql"
	"github.com/aperturerobotics/hydra/volume"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	sr.AddFactory(reconciler_example.NewFactory(b))

	verbose := true
	/*
		av, _, ref, err := common.AddStorageVolume(ctx, le, b, sr, verbose)
		if err != nil {
			panic(err)
		}
	*/
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

	le.Info("storage volume resolved")
	volCtr := av.(volume.Controller)
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		panic(err)
	}

	bucketID := "test-bucket-mysql"
	volID := vol.GetID()
	_, _, _, err = vol.ApplyBucketConfig(&bucket.Config{
		Id:  bucketID,
		Rev: 1,
	})
	if err != nil {
		panic(err)
	}

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
		tx, err := sq.NewMysqlTransaction(true)
		if err != nil {
			panic(err)
		}
		// assert that the database exists
		_, err = tx.OpenDatabase(dbName, true)
		if err != nil {
			panic(err)
		}
		db, sqlDB, err := mysql.NewMysqlGorm(
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

	tx, db, _ := buildTx(true)
	if err := db.AutoMigrate(&Entry{}); err != nil {
		panic(err)
	}
	createVals := []*Entry{
		{Value: 4, ID: 1},
		{Value: 10, ID: 2},
		{Value: 30, ID: 3},
	}
	for _, v := range createVals {
		db.Create(v)
	}
	// db = db.Commit()

	// TODO Find() does not work before Commit - why?
	err = tx.Commit(ctx)
	if err != nil {
		panic(err)
	}
	le.Infof("successfully stored %d objects", 3)

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

	var e Entry
	out = db.Where("value = ?", 30).Find(&e)
	if out.Error != nil {
		panic(out.Error)
	}
	if e.Value != 30 {
		panic("value was incorrect")
	}
	le.Infof("successfully retrieved object by value lookup: %#v", e)

	tx.Discard()
}

// Entry is an entry in the database.
type Entry struct {
	ID    int `gorm:"primaryKey"`
	Value int `json:"value"`
}

var _ interface{} = common.AddStorageVolume
