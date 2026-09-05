package publisher

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	db_core "github.com/s4wave/spacewave/db/core"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// LocalWorld owns a plaintext release World mounted from a Bldr publication.
// Release must follow all transactions and publication operations.
type LocalWorld struct {
	// Engine is the mounted release World.
	Engine world.Engine
	// release unwinds controller attachments in reverse construction order.
	release []func()
}

// OpenLocalWorld mounts an application's dedicated publication database.
// Identifiers are process-local aliases, not cloud accounts or Space IDs.
func OpenLocalWorld(ctx context.Context, le *logrus.Entry, boltPath, engineID, bucketID string) (_ *LocalWorld, rerr error) {
	// Require an explicit publication destination before opening local storage.
	if boltPath == "" || engineID == "" || bucketID == "" {
		return nil, errors.New("release database, engine, and bucket are required")
	}
	absPath, err := filepath.Abs(boltPath)
	if err != nil {
		return nil, err
	}

	// Keep the bus lifetime with its attached storage and World controllers.
	ctx, cancel := context.WithCancel(ctx)
	out := &LocalWorld{release: []func(){cancel}}
	defer func() {
		if rerr != nil {
			out.Release()
		}
	}()
	b, sr, err := db_core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	sr.AddFactory(world_block_engine.NewFactory(b))
	_, _, volumeRef, err := loader.WaitExecControllerRunning(ctx, b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{
			Path: absPath, NoWriteKey: true,
			VolumeConfig: &volume_controller.Config{VolumeIdAlias: []string{"release-volume"}},
		}), nil)
	if err != nil {
		return nil, err
	}
	out.release = append(out.release, volumeRef.Release)
	_, _, nodeRef, err := loader.WaitExecControllerRunning(ctx, b,
		resolver.NewLoadControllerWithConfig(&node_controller.Config{}), nil)
	if err != nil {
		return nil, err
	}
	out.release = append(out.release, nodeRef.Release)

	// Match the Bldr remote's plaintext bucket and World object-store aliases.
	conf, err := bucket.NewConfig(bucketID, 1, nil)
	if err != nil {
		return nil, err
	}
	if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(conf, "release-volume")); err != nil {
		return nil, err
	}
	ctrl, ref, err := world_block_engine.StartEngineWithConfig(ctx, b,
		world_block_engine.NewConfig(engineID, "release-volume", bucketID, bucketID, nil, nil, false))
	if err != nil {
		return nil, err
	}
	out.release = append(out.release, ref.Release)
	out.Engine, err = ctrl.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Release closes the local World and storage after its consumers have stopped.
func (w *LocalWorld) Release() {
	for _, v := range slices.Backward(w.release) {
		v()
	}
	w.release = nil
}
