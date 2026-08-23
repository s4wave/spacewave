package lean

import (
	"context"
	"fmt"
	"os"

	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_overlay "github.com/s4wave/spacewave/db/block/store/overlay"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	world "github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	volume_filesnap "github.com/s4wave/spacewave/prototypes/sync-library/lean/filesnap"
	"github.com/sirupsen/logrus"
)

// OpenWorldDurable constructs a Hydra world backed by a durable snapshot
// file volume. Same factory set as OpenWorld with the in-memory volume
// swapped for the file-backed store; all state survives Close and a later
// reopen at the same path.
func OpenWorldDurable(ctx context.Context, dir string) (*World, error) {
	log := logrus.New()
	if os.Getenv("KV_DEBUG") != "" {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.ErrorLevel)
	}
	return openWorldDurableLogged(ctx, dir, log)
}

func openWorldDurableLogged(ctx context.Context, dir string, log *logrus.Logger) (*World, error) {
	le := logrus.NewEntry(log)

	ctx, cancel := context.WithCancel(ctx)
	w := &World{ctx: ctx, cancel: cancel}
	fail := func(err error) (*World, error) {
		w.Close()
		return nil, err
	}

	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return fail(err)
	}
	w.Bus = b
	w.StaticResolver = sr
	sr.AddFactory(bucket_setup.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(volume_filesnap.NewFactory(b))
	sr.AddFactory(block_store_inmem.NewFactory(b))
	sr.AddFactory(block_store_overlay.NewFactory(b))
	sr.AddFactory(boilerplate_controller.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	if _, _, _, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	); err != nil {
		return fail(fmt.Errorf("configset controller: %w", err))
	}
	if _, _, _, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&node_controller.Config{}),
		nil,
	); err != nil {
		return fail(fmt.Errorf("node controller: %w", err))
	}

	dv, _, volRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_filesnap.Config{Path: durablePath(dir)}),
		nil,
	)
	if err != nil {
		return fail(fmt.Errorf("volume controller: %w", err))
	}
	w.rels = append(w.rels, volRef.Release)
	vc := dv.(volume.Controller)
	v, err := vc.GetVolume(ctx)
	if err != nil {
		return fail(fmt.Errorf("get volume: %w", err))
	}
	if _, _, _, err := v.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  engineBucketID,
		Rev: 1,
	}); err != nil {
		return fail(fmt.Errorf("apply bucket config: %w", err))
	}

	transformConf, err := engineTransformConfig(engineBucketID)
	if err != nil {
		return fail(fmt.Errorf("build transform config: %w", err))
	}
	initRef := &bucket.ObjectRef{
		BucketId:      engineBucketID,
		TransformConf: transformConf,
	}
	engConf := world_block_engine.NewConfig(
		"lean-engine",
		v.GetID(), engineBucketID,
		"lean-engine-store",
		initRef,
		nil,
		false,
	)
	_, ctrlRef, err := world_block_engine.StartEngineWithConfig(ctx, b, engConf)
	if err != nil {
		return fail(fmt.Errorf("start world engine: %w", err))
	}
	w.rels = append(w.rels, ctrlRef.Release)

	busEngine := world.NewBusEngine(ctx, b, "lean-engine")
	w.WS = world.NewEngineWorldState(busEngine, true)
	return w, nil
}

// durablePath returns the snapshot file path for a durable root dir.
func durablePath(dir string) string {
	return dir + "/kv-durable.json"
}
