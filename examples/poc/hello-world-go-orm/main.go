package main

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/core"
	common "github.com/aperturerobotics/hydra/examples/common"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/sql/genji"
	"github.com/aperturerobotics/hydra/volume"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Note: currently broken, until this is fixed:
// https://github.com/genjidb/genji/issues/383
// The sql driver in Scan() tries to convert lazilyLoadedDocument to sql.Rows and fails.

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

	// Run the go-orm demo.
	vol, err := volCtr.GetVolume(ctx)
	if err != nil {
		panic(err)
	}
	obs, err := vol.OpenObjectStore(ctx, "go-orm-demo")
	if err != nil {
		panic(err)
	}
	db, err := kvtx_genji.NewKvtxGorm(ctx, le, obs, &gorm.Config{})
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&Entry{}); err != nil {
		panic(err)
	}
	db.Create(&Entry{Value: 4, ID: 1})
	db.Create(&Entry{Value: 10, ID: 2})
	db.Create(&Entry{Value: 30, ID: 3})

	var e Entry
	out := db.Where("value = ?", 30).Find(&e)
	if out.Error != nil {
		panic(out.Error)
	}
	if e.Value != 30 {
		panic(errors.New("value was incorrect"))
	}
}

// Entry is an entry in the database.
type Entry struct {
	ID    int `gorm:"primaryKey"`
	Value int `json:"value"`
}

var _ interface{} = common.AddStorageVolume
