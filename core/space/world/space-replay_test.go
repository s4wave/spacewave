package space_world

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	bquad "github.com/s4wave/spacewave/db/block/quad"
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
	}
	r := &spaceReplay{a: a, stage: newReplayStage(a.pages)}
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

// changeOn commits change on prev through the replay, failing the test on a
// refusal.
func changeOn(ctx context.Context, t *testing.T, r *spaceReplay, prev *InnerState, change func(*world_block.EngineTx) error) *InnerState {
	t.Helper()
	next, reason, err := r.commitOn(ctx, prev, "change failed", func(btx *world_block.EngineTx) (bool, error) {
		err := change(btx)
		return err == nil, err
	})
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	return next
}

// quadsFrom returns the quads leaving each of keys in the World state, each
// as "subject predicate object".
func quadsFrom(ctx context.Context, t *testing.T, r *spaceReplay, state *InnerState, keys ...string) []string {
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
	var out []string
	for _, key := range keys {
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, "", "", ""), 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range quads {
			out = append(out, q.GetSubject()+" "+q.GetPredicate()+" "+q.GetObj())
		}
	}
	slices.Sort(out)
	return out
}

// TestRegenerateWorldKeepsEveryRecordAndQuad drives a Space World through a
// regeneration, a switch back and a second regeneration, trying to make each
// step lose or invent a record or a quad: objects deleted and created again
// around the operation, a carried quad the World already holds, a write after
// the switch that sets a carried quad again, a stale release, and a step that
// cannot finish. It fails when the build drops or adds a quad, when a switch
// back loses a write ordered after the regeneration, when a release frees
// roots other than the ones it names, or when a step that could not finish is
// refused instead of stopping replay.
func TestRegenerateWorldKeepsEveryRecordAndQuad(t *testing.T) {
	ctx := t.Context()
	r, start := newWorldReplay(t)
	r.a.opts.Regen.SpillDir = t.TempDir()
	keys := []string{"a", "b", "c", "d"}
	format := func(state *InnerState) world_block.WorldFormat {
		t.Helper()
		got, err := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig()).Format(ctx, state.GetHeadRef().GetRootRef())
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	link := func(subj, pred, obj string) world.GraphQuad {
		return world.NewGraphQuadWithKeys(subj, "<"+pred+">", obj, "")
	}

	// Build a kv World: c deleted and created again, quads into b and c.
	kv := createOn(ctx, t, r, start, "a")
	kv = createOn(ctx, t, r, kv, "b")
	kv = createOn(ctx, t, r, kv, "c")
	kv = changeOn(ctx, t, r, kv, func(btx *world_block.EngineTx) error {
		_, err := btx.DeleteObject(ctx, "c")
		return err
	})
	kv = createOn(ctx, t, r, kv, "c")
	kv = changeOn(ctx, t, r, kv, func(btx *world_block.EngineTx) error {
		return errors.Join(btx.SetGraphQuad(ctx, link("a", "p", "b")), btx.SetGraphQuad(ctx, link("a", "p", "c")))
	})

	// Carry a quad the World holds and one it lacks.
	carried := []*bquad.Quad{world.GraphQuadToQuad(link("a", "p", "b")), world.GraphQuadToQuad(link("a", "q", "b"))}
	slices.SortFunc(carried, CompareQuads)
	btx, bcs := r.cursor.BuildTransactionWithStore(nil, r.store())
	err := WriteQuadList(ctx, bcs, func(cb func(*bquad.Quad) error) error {
		for _, q := range carried {
			if err := cb(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	list, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	forward := &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH, Quads: list}

	// A step that cannot finish stops replay and refuses nothing.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, reason, err := r.applyRegenerateWorld(canceled, forward, kv); reason != "" || !isReplayGap(err) {
		t.Fatalf("a canceled regeneration returned reason %q and error %v, want a replay gap", reason, err)
	}

	// Regenerate: the node graph holds every record and quad, and the
	// carried one, with base and built held.
	graph, reason, err := r.applyRegenerateWorld(ctx, forward, kv)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	if got := format(graph); got != world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		t.Fatalf("the regenerated World is %v", got)
	}
	base, built := heldRegen(graph)
	if base == nil || !built.EqualsRef(graph.GetHeadRef().GetRootRef()) {
		t.Fatalf("the regeneration holds base %v and built %v", base, built)
	}
	if err := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig()).Compare(ctx, base, built); err != nil {
		t.Fatal(err)
	}
	want := []string{"<a> <p> <b>", "<a> <p> <c>", "<a> <q> <b>"}
	if got := quadsFrom(ctx, t, r, graph, keys...); !slices.Equal(got, want) {
		t.Fatalf("the regenerated World holds %v, want %v", got, want)
	}
	if got := hasKeys(ctx, t, r, graph, keys...); !got[0] || !got[1] || !got[2] || got[3] {
		t.Fatalf("the regenerated World holds a, b, c, d = %v", got)
	}

	// Every replica builds the same World, and none regenerates it twice or
	// names its roots outside the operation.
	again, reason, err := r.applyRegenerateWorld(ctx, forward, kv)
	if err != nil || reason != "" || !again.EqualVT(graph) {
		t.Fatalf("a second replica's regeneration gave %v, %q, %v", again.GetHeadRef(), reason, err)
	}
	if _, reason, _ := r.applyRegenerateWorld(ctx, forward, graph); reasonCode(reason) != "world regeneration" {
		t.Fatalf("a held regeneration's second run gave reason %q", reason)
	}
	if _, reason := applySetRetainedRoot(&SetRetainedRootOp{Name: regenBaseName}, graph); reason != reasonReservedRoot {
		t.Fatalf("a retained root op on a regeneration's name gave reason %q", reason)
	}

	// Write on the node graph: set the carried quad again, drop a quad, add
	// d with a quad, and delete and create c again.
	graph = changeOn(ctx, t, r, graph, func(btx *world_block.EngineTx) error {
		return errors.Join(btx.SetGraphQuad(ctx, link("a", "q", "b")), btx.DeleteGraphQuad(ctx, link("a", "p", "c")))
	})
	graph = createOn(ctx, t, r, graph, "d")
	graph = changeOn(ctx, t, r, graph, func(btx *world_block.EngineTx) error {
		return btx.SetGraphQuad(ctx, link("d", "p", "a"))
	})

	// A release naming other roots frees nothing.
	if _, reason := applyReleaseWorldRegen(&ReleaseWorldRegenOp{Base: base, Built: base}, graph); reason == "" {
		t.Fatal("a stale release was accepted")
	}

	// Switch back: the kv World keeps every write ordered after the
	// regeneration, the carried quad included, and holds nothing.
	backOp := &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV}
	back, reason, err := r.applyRegenerateWorld(ctx, backOp, graph)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	if got := format(back); got != world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		t.Fatalf("the switched-back World is %v", got)
	}
	if b, n := heldRegen(back); b != nil || n != nil {
		t.Fatal("the switch back still holds the regeneration")
	}
	want = []string{"<a> <p> <b>", "<a> <q> <b>", "<d> <p> <a>"}
	if got := quadsFrom(ctx, t, r, back, keys...); !slices.Equal(got, want) {
		t.Fatalf("the switched-back World holds %v, want %v", got, want)
	}
	if got := hasKeys(ctx, t, r, back, keys...); slices.Contains(got, false) {
		t.Fatalf("the switched-back World holds a, b, c, d = %v", got)
	}

	// Regenerate again and release: both roots go, and the World stays.
	graph, reason, err = r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH}, back)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	base, built = heldRegen(graph)
	released, reason := applyReleaseWorldRegen(&ReleaseWorldRegenOp{Base: base, Built: built}, graph)
	if reason != "" {
		t.Fatal(reason)
	}
	if b, n := heldRegen(released); b != nil || n != nil || !released.GetHeadRef().EqualVT(graph.GetHeadRef()) {
		t.Fatal("the release kept a root or moved the World")
	}
	if got := quadsFrom(ctx, t, r, released, keys...); !slices.Equal(got, want) {
		t.Fatalf("the released World holds %v, want %v", got, want)
	}
}
