package space_world

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// newWorldReplay returns a transient replay over an in-memory Volume that
// runs World transactions, and the World it initialized.
func newWorldReplay(t *testing.T) (*spaceReplay, *InnerState) {
	// Open the bucket.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: time.Hour.String()},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)

	// Build the adapter and its replay.
	a := &Adapter{
		le:       logrus.NewEntry(log),
		opts:     AdapterOptions{Bus: tb.Bus, StepFactorySet: tb.StepFactorySet},
		bucketID: tb.BucketId,
		pages:    block.NewPendingStore(cursor.GetBucket(), 1<<20),
		decoded:  block.NewDecodedBlockCache(),
		now:      time.Now,
	}
	r := &spaceReplay{a: a}
	t.Cleanup(r.close)

	// Initialize a World; its first commit writes its root.
	conf, err := buildDefaultTransformConf()
	if err != nil {
		t.Fatal(err)
	}
	start, reason, err := r.applyInit(ctx, &InitWorldOp{TransformConf: conf}, &InnerState{})
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	return r, start
}

// createOn commits a World transaction on prev that creates key.
func createOn(ctx context.Context, t *testing.T, r *spaceReplay, prev *InnerState, key string) *InnerState {
	t.Helper()
	next, reason, err := r.commitOn(ctx, prev, "create failed", func(btx *world_block.EngineTx) (bool, error) {
		obj, err := btx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		return err == nil, err
	})
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	return next
}

// hasKeys reports which of keys the World state holds, on the replay engine.
func hasKeys(ctx context.Context, t *testing.T, r *spaceReplay, state *InnerState, keys ...string) []bool {
	t.Helper()
	engine, err := r.bind(ctx, state.GetHeadRef())
	if err != nil {
		t.Fatal(err)
	}
	tx, err := engine.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	out := make([]bool, len(keys))
	for i, key := range keys {
		var obj world.ObjectState
		obj, out[i], err = tx.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestReplayMovesEngineToAnotherWorld checks that a replay whose next
// operation runs on another World than its engine serves, as after an adopted
// own candidate or a reorder, moves the engine there rather than building a
// new one, and that the operation then sees that World, not the one the
// engine served. It fails when bind rebuilds the engine for another World.
func TestReplayMovesEngineToAnotherWorld(t *testing.T) {
	ctx := t.Context()
	r, start := newWorldReplay(t)

	// Build a then b on the initial World.
	first := createOn(ctx, t, r, start, "a")
	engine := r.engine
	second := createOn(ctx, t, r, first, "b")
	if r.engine != engine {
		t.Fatal("a commit on the engine's own head rebuilt the engine")
	}

	// Branch from the first World: the engine moves back to it.
	branch := createOn(ctx, t, r, first, "c")
	if r.engine != engine {
		t.Fatal("an operation on another World rebuilt the replay engine")
	}
	if got := hasKeys(ctx, t, r, branch, "a", "b", "c"); !got[0] || got[1] || !got[2] {
		t.Fatalf("the branch holds a, b, c = %v, want true, false, true", got)
	}
	if got := hasKeys(ctx, t, r, second, "a", "b", "c"); !got[0] || !got[1] || got[2] {
		t.Fatalf("the second World holds a, b, c = %v, want true, true, false", got)
	}
	if r.engine != engine {
		t.Fatal("reading another World rebuilt the replay engine")
	}
}

// TestCursorWaitsPastACheckpoint checks that the adapter stores no replay
// cursor while the positions above the checkpoint are only what the next
// checkpoint covers and the unstable ones after it, and saves it once they
// run replayCursorOperations past it. It fails when the cursor saves within
// the checkpoint cadence, which stores a World between every two checkpoints.
func TestCursorWaitsPastACheckpoint(t *testing.T) {
	f := newReplayFixture(t, 1<<20)
	a, ctx := f.a, t.Context()

	// The stable operations of the next checkpoint and as many unstable ones
	// store nothing.
	for i := range 2*sobject.MinCheckpointOperations - 1 {
		name := "position " + strconv.Itoa(i)
		f.apply(name, f.put(name, 0))
		if err := a.maybeSaveCursor(ctx); err != nil {
			t.Fatal(err)
		}
	}
	a.waitBackground()
	if a.replay.saved != 0 {
		t.Fatalf("the cursor saved at %d positions within the checkpoint cadence", a.replay.saved)
	}

	// The next position, with no checkpoint, saves it.
	f.apply("last", f.put("last", 0))
	if err := a.maybeSaveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	a.waitBackground()
	if want := len(a.replay.positions); a.replay.saved != want {
		t.Fatalf("the cursor saved %d positions, want %d", a.replay.saved, want)
	}
}
