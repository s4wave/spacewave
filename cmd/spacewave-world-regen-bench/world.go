//go:build !js && !wasip1

package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// benchEngineID is the World engine's ID. Its head root,
// world_block.WorldRootName(benchEngineID), is "world:regen-bench", the root
// every earlier seed holds.
const benchEngineID = "regen-bench"

// benchObjectStoreID is the object store through which the engine finds the
// Volume's coordinator, as every writable local World names one.
const benchObjectStoreID = "regen-bench-store"

// profile is one deployment's store, collector and cache settings for the
// World's Volume.
type profile struct {
	// volume returns the Volume config for the database file at path.
	volume func(path string) config.Config
	// factory constructs the Volume, or nil for the factory the bus resolves.
	factory func(b bus.Bus) controller.Factory
	// decodedCacheBytes is the process-wide decoded-block cache's budget.
	decodedCacheBytes int64
}

// Hyperline's World store settings, from its backend/world-store.go and the
// cache size its deploy/compose.yaml sets.
const (
	// hyperlineTable is worldSQLiteTable.
	hyperlineTable = "world"
	// hyperlineMmapBytes is worldSQLiteMmapBytes.
	hyperlineMmapBytes = 64 << 30
	// hyperlineWALBytes is worldSQLiteWALBytes, the WAL bound its
	// openWorldSQLite (backend/world-node-push.go) opens the World with.
	hyperlineWALBytes = 512 << 20
	// hyperlineDecodedCacheBytes is HYPERLINE_BLOCK_CACHE_MIB, 1536.
	hyperlineDecodedCacheBytes = 1536 << 20
)

// profiles are the deployments the bench opens the World as.
var profiles = map[string]profile{
	// spacewave is a native Spacewave app's: an s4db Volume as
	// bldr/storage/native/s4db.go builds it, with the empty volume controller
	// config a storage Volume gets (cmd/spacewave-store-bench/storebench), so
	// the default collector, and db/block's default decoded cache.
	"spacewave": {
		volume: func(path string) config.Config {
			return &volume_s4db.Config{Path: path, VolumeConfig: &volume_controller.Config{}}
		},
		decodedCacheBytes: block.DefaultDecodedBlockCacheMaxCost,
	},
	// hyperline is Hyperline's World server's: a SQLite Volume configured as
	// its worldSQLiteConfig does, with the sweep off as its world-store.go
	// sets GcIntervalDur "0", opened with its WAL bound, and its decoded
	// cache.
	"hyperline": {
		volume: func(path string) config.Config {
			return &volume_sqlite.Config{
				Path:         path,
				Table:        hyperlineTable,
				MmapSize:     hyperlineMmapBytes,
				VolumeConfig: &volume_controller.Config{GcIntervalDur: "0"},
			}
		},
		factory: func(b bus.Bus) controller.Factory {
			return volume_sqlite.NewFactoryWithOpener(b, func(ctx context.Context, le *logrus.Entry, c *volume_sqlite.Config) (volume.Volume, error) {
				return volume_sqlite.NewSqliteWithOptions(ctx, le, c, volume_sqlite.Options{WALCheckpointBytes: hyperlineWALBytes})
			})
		},
		decodedCacheBytes: hyperlineDecodedCacheBytes,
	},
}

// profileNames lists the profiles for the flag's usage.
const profileNames = "spacewave or hyperline"

// regenEvent is one regeneration log line of the engine controller.
type regenEvent struct {
	// msg is the line's message.
	msg string
	// stage is its regen-stage field.
	stage string
	// took is its took field, set on a step.
	took string
	// err is its error, set when the regeneration stopped.
	err error
}

// logHook prints warnings on stderr and passes the engine controller's
// regeneration lines to regen, dropping them when nothing reads.
type logHook struct {
	// regen receives the regeneration lines.
	regen chan regenEvent
}

// Levels returns the levels the hook fires on.
func (h *logHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire prints a warning and forwards a regeneration line.
func (h *logHook) Fire(e *logrus.Entry) error {
	// Print warnings and errors.
	if e.Level <= logrus.WarnLevel {
		if line, err := e.String(); err == nil {
			_, _ = os.Stderr.WriteString(line)
		}
	}

	// Forward the regeneration's progress.
	if !strings.HasPrefix(e.Message, "world regeneration") {
		return nil
	}
	ev := regenEvent{msg: e.Message}
	ev.stage, _ = e.Data["regen-stage"].(string)
	ev.took, _ = e.Data["took"].(string)
	ev.err, _ = e.Data[logrus.ErrorKey].(error)
	select {
	case h.regen <- ev:
	default:
	}
	return nil
}

// benchWorld is the bench's World as production opens a local one: the World
// engine controller on a bus over the profile's Volume, its writers through
// the engine the bus resolves. The bench reads the regeneration's stage
// through its own authority over the same roots and never writes through it.
type benchWorld struct {
	tb      *testbed.Testbed
	hook    *logHook
	base    *bucket_lookup.Cursor
	observe *world_block.LocalAuthority

	// mtx pauses the writers while the controller restarts, and guards the
	// fields below.
	mtx sync.RWMutex
	// ref holds the running engine controller.
	ref directive.Reference
	// engine is the writers' handle on the running controller's engine.
	engine *world.BusEngine
	// cancel ends engine's lookup.
	cancel context.CancelFunc
}

// openBenchWorld opens the World in the database file at path as the named
// profile's deployment does, creating both when absent, and starts its
// engine controller with no regeneration. The testbed's bucket ID is fixed,
// so a reopen finds the same root.
func openBenchWorld(ctx context.Context, path, name string) (*benchWorld, error) {
	// Choose the profile and size the decoded cache before any engine opens.
	p, ok := profiles[name]
	if !ok {
		return nil, errors.Errorf("unknown config %q: want %s", name, profileNames)
	}
	block.SetSharedDecodedBlockCacheMaxCost(p.decodedCacheBytes)

	// Log only to the hook: warnings to stderr, regeneration lines to run.
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	log.SetOutput(io.Discard)
	hook := &logHook{regen: make(chan regenEvent, 64)}
	log.AddHook(hook)
	le := logrus.NewEntry(log)

	// Open the Volume and the World's bucket on it.
	opts := []testbed.Option{testbed.WithVolumeConfig(p.volume(path))}
	if p.factory != nil {
		opts = append(opts, testbed.WithVolumeFactory(p.factory))
	}
	tb, err := testbed.NewTestbed(ctx, le, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "open volume")
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Observe the World's roots through an authority of the bench's own.
	w := &benchWorld{tb: tb, hook: hook}
	if w.base, err = tb.BuildEmptyCursor(ctx); err != nil {
		w.Close()
		return nil, err
	}
	name = world_block.WorldRootName(benchEngineID)
	scope := coord.Scope{VolumeID: tb.Volume.GetID(), ObjectStoreID: name, ParticipantID: "bench"}
	w.observe = world_block.NewLocalAuthority(le, w.base, name, nil, scope)

	// Start the engine and the writers' handle on it.
	if err := w.start(ctx, nil); err != nil {
		w.Close()
		return nil, err
	}
	return w, nil
}

// start starts the engine controller with the regeneration config, nil for
// none, waits for its engine, and gives the writers a fresh handle on it. The
// config is a writable local World's, as core/resource/root/controller/app.go
// builds one: the bucket's head under the engine's root, the changelog kept,
// and the old graph format for a new World.
func (w *benchWorld) start(ctx context.Context, regen *world_block_engine.RegenConfig) error {
	// Configure the World with the regeneration.
	bucketID := w.tb.BucketId
	conf := world_block_engine.NewConfig(
		benchEngineID,
		w.tb.Volume.GetID(), bucketID,
		benchObjectStoreID,
		&bucket.ObjectRef{BucketId: bucketID},
		nil,
		true,
	)
	conf.Regen = regen

	// Load the controller and wait for its engine.
	_, ref, err := world.StartEngine(ctx, w.tb.Bus, conf, nil)
	if err != nil {
		return errors.Wrap(err, "start world engine")
	}
	w.ref = ref

	// Hand the writers a handle that resolves the new engine. A handle kept
	// across the restart can still hold the closed engine, which the
	// controller closes rather than retires, so its next commit fails.
	engineCtx, cancel := context.WithCancel(ctx)
	w.engine, w.cancel = world.NewBusEngine(engineCtx, w.tb.Bus, benchEngineID), cancel
	return nil
}

// restart restarts the engine controller with the regeneration config, as an
// operator restarts a server with a new target, pausing the writers.
func (w *benchWorld) restart(ctx context.Context, regen *world_block_engine.RegenConfig) error {
	// Watch the running engine for its removal from the bus.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	dropped := make(chan struct{})
	var drop sync.Once
	_, _, lookup, err := world.ExLookupWorldEngine(ctx, w.tb.Bus, false, benchEngineID, func() {
		drop.Do(func() { close(dropped) })
	})
	if err != nil {
		return errors.Wrap(err, "look up world engine")
	}
	defer lookup.Release()

	// Stop the running controller and the writers' handle once no commit is
	// in flight, and wait until the bus drops the engine. Its release is
	// asynchronous, so a successor started sooner resolves the closing
	// engine, and so does the writers' new handle.
	w.cancel()
	w.ref.Release()
	w.ref = nil
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-dropped:
	}

	// Start its successor with the target.
	return w.start(ctx, regen)
}

// stage reads the regeneration's stage from the World's roots.
func (w *benchWorld) stage(ctx context.Context) (world_block.RegenStage, error) {
	return world_block.NewRegenerator(w.observe, world_block.RegenConfig{}).Stage(ctx)
}

// Close stops the engine controller and closes the Volume.
func (w *benchWorld) Close() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.ref != nil {
		w.ref.Release()
	}
	if w.base != nil {
		w.base.Release()
	}
	w.tb.Release()
}
