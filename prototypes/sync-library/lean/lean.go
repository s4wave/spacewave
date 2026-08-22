package lean

import (
	"context"
	"fmt"

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
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	world "github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// engineBucketID is the bucket the lean world engine uses.
const engineBucketID = "test-bucket"

// RunLean constructs a Hydra world without any transport factory and runs the
// object and graph checks against it.
func RunLean(ctx context.Context) error {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	le := logrus.NewEntry(log)

	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	sr.AddFactory(bucket_setup.NewFactory(b))
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
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
		return fmt.Errorf("configset controller: %w", err)
	}
	if _, _, _, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&node_controller.Config{}),
		nil,
	); err != nil {
		return fmt.Errorf("node controller: %w", err)
	}

	dv, _, volRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_kvtxinmem.Config{}),
		nil,
	)
	if err != nil {
		return fmt.Errorf("volume controller: %w", err)
	}
	defer volRef.Release()
	vc := dv.(volume.Controller)
	v, err := vc.GetVolume(ctx)
	if err != nil {
		return fmt.Errorf("get volume: %w", err)
	}
	if _, _, _, err := v.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  engineBucketID,
		Rev: 1,
	}); err != nil {
		return fmt.Errorf("apply bucket config: %w", err)
	}

	transformConf, err := engineTransformConfig(engineBucketID)
	if err != nil {
		return fmt.Errorf("build transform config: %w", err)
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
		return fmt.Errorf("start world engine: %w", err)
	}
	defer ctrlRef.Release()

	busEngine := world.NewBusEngine(ctx, b, "lean-engine")
	ws := world.NewEngineWorldState(busEngine, true)
	return checkWorld(ctx, ws)
}
