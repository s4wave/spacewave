package space_world

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	bquad "github.com/s4wave/spacewave/db/block/quad"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/sirupsen/logrus"
)

// newWorldReplay returns a transient replay over an in-memory Volume that
// runs World transactions, and the World it initialized.
func newWorldReplay(t *testing.T) (*spaceReplay, *InnerState) {
	conf, err := buildDefaultTransformConf()
	if err != nil {
		t.Fatal(err)
	}
	return newWorldReplayOn(t, conf, nil, 1<<20)
}

// newWorldReplayOn returns a replay as newWorldReplay does, of a World with
// the transform conf, whose kept pages past bound bytes go to the Volume
// through wrap when it is set.
func newWorldReplayOn(t *testing.T, conf *block_transform.Config, wrap func(block.StoreOps) block.StoreOps, bound int) (*spaceReplay, *InnerState) {
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
	var store block.StoreOps = cursor.GetBucket()
	if wrap != nil {
		store = wrap(store)
	}

	// Build the adapter and its replay.
	a := &Adapter{
		le:       logrus.NewEntry(log),
		opts:     AdapterOptions{Bus: tb.Bus, StepFactorySet: tb.StepFactorySet},
		bucketID: tb.BucketId,
		pages:    block.NewPendingStore(store, bound),
		decoded:  block.NewDecodedBlockCache(),
	}
	a.regenBuild = newRegenBuilder(a)
	r := &spaceReplay{a: a, stage: newReplayStage(a.pages)}
	t.Cleanup(r.close)

	// Initialize a World; its first commit writes its root.
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
		got, err := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig(regenSpillReplay)).Format(ctx, state.GetHeadRef().GetRootRef())
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

	// Carry a quad the World holds and one it lacks, as the engine writes
	// them before the pin.
	uncarried := kv
	kv = carryOn(ctx, t, r, kv, link("a", "p", "b"), link("a", "q", "b"))

	// Pin and switch: the node graph holds every record and quad, and the
	// carried one, with the head at the switch as base and built held and
	// the pin released.
	pinned, graph := regenOn(ctx, t, r, kv)

	// A switch that cannot finish stops replay and refuses nothing.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	pinRoot, _, _ := heldRegen(pinned)
	switchOp := &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH, Base: pinRoot}
	if _, reason, err := r.applyRegenerateWorld(canceled, switchOp, pinned); reason != "" || !isReplayGap(err) {
		t.Fatalf("a canceled switch returned reason %q and error %v, want a replay gap", reason, err)
	}
	if got := format(pinned); got != world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		t.Fatalf("the pinned World is %v", got)
	}
	if got := format(graph); got != world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		t.Fatalf("the regenerated World is %v", got)
	}
	pin, base, built := heldRegen(graph)
	if pin != nil || !base.EqualsRef(pinned.GetHeadRef().GetRootRef()) || !built.EqualsRef(graph.GetHeadRef().GetRootRef()) {
		t.Fatalf("the regeneration holds pin %v, base %v and built %v", pin, base, built)
	}
	if err := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig(regenSpillReplay)).Compare(ctx, base, built); err != nil {
		t.Fatal(err)
	}
	want := []string{"<a> <p> <b>", "<a> <p> <c>", "<a> <q> <b>"}
	if got := quadsFrom(ctx, t, r, graph, keys...); !slices.Equal(got, want) {
		t.Fatalf("the regenerated World holds %v, want %v", got, want)
	}
	if got := hasKeys(ctx, t, r, graph, keys...); !got[0] || !got[1] || !got[2] || got[3] {
		t.Fatalf("the regenerated World holds a, b, c, d = %v", got)
	}

	// Every replica builds the same World, whether it built the pin ahead
	// or holds a build of another pin, and none regenerates it twice or
	// names its roots outside the operation.
	if _, again := regenOn(ctx, t, r, kv); !again.EqualVT(graph) {
		t.Fatalf("a second replica's regeneration gave %v", again.GetHeadRef())
	}
	r.a.regenBuild.set(&regenBuild{pin: uncarried.GetHeadRef().GetRootRef(), built: uncarried.GetHeadRef().GetRootRef()})
	if again, reason, err := r.applyRegenerateWorld(ctx, switchOp, pinned); err != nil || reason != "" || !again.EqualVT(graph) {
		t.Fatalf("a replica holding another pin's build gave %v, %q, %v", again.GetHeadRef(), reason, err)
	}
	prebuilt, err := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig(regenSpillReplay)).Build(ctx, pinRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.a.regenBuild.set(&regenBuild{pin: pinRoot, built: prebuilt})
	if again, reason, err := r.applyRegenerateWorld(ctx, switchOp, pinned); err != nil || reason != "" || !again.EqualVT(graph) {
		t.Fatalf("a replica with the pin prebuilt gave %v, %q, %v", again.GetHeadRef(), reason, err)
	}
	r.a.regenBuild.set(nil)

	// A switch naming another pin is refused; TestRegenerateWorldReplicasWhileWriting
	// refuses one past the bound.
	stale := &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH, Base: uncarried.GetHeadRef().GetRootRef()}
	if _, reason, _ := r.applyRegenerateWorld(ctx, stale, pinned); reason != reasonRegenPinDiffers {
		t.Fatalf("a switch naming another pin gave reason %q", reason)
	}
	if _, reason, _ := r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH}, graph); reasonCode(reason) != "world regeneration" {
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
	if p, b, n := heldRegen(back); p != nil || b != nil || n != nil {
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
	_, graph = regenOn(ctx, t, r, back)
	_, base, built = heldRegen(graph)
	released, reason := applyReleaseWorldRegen(&ReleaseWorldRegenOp{Base: base, Built: built}, graph)
	if reason != "" {
		t.Fatal(reason)
	}
	if p, b, n := heldRegen(released); p != nil || b != nil || n != nil || !released.GetHeadRef().EqualVT(graph.GetHeadRef()) {
		t.Fatal("the release kept a root or moved the World")
	}
	if got := quadsFrom(ctx, t, r, released, keys...); !slices.Equal(got, want) {
		t.Fatalf("the released World holds %v, want %v", got, want)
	}
}

// regenOn pins prev and switches the pin to the node graph, as every replica
// applies the two operations.
func regenOn(ctx context.Context, t *testing.T, r *spaceReplay, prev *InnerState) (pinned, graph *InnerState) {
	t.Helper()
	target := world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH
	pinned, reason, err := r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: target}, prev)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	pin, _, _ := heldRegen(pinned)
	if !pin.EqualsRef(pinned.GetHeadRef().GetRootRef()) {
		t.Fatalf("the pin %v is not the pinned head", pin)
	}
	graph, reason, err = r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: target, Base: pin}, pinned)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	return pinned, graph
}

// carryOn applies quads to prev as the engine writes carried quads: one
// SetGraphQuads operation, which replay applies in one graph update.
func carryOn(ctx context.Context, t *testing.T, r *spaceReplay, prev *InnerState, quads ...world.GraphQuad) *InnerState {
	t.Helper()
	next, reason, err := r.applySetGraphQuads(ctx, carryList(quads...), prev)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	return next
}

// carryList returns the graph quad list that sets quads in order.
func carryList(quads ...world.GraphQuad) *world_block_tx.TxSetGraphQuads {
	list := make([]*bquad.Quad, len(quads))
	for i, q := range quads {
		list[i] = world.GraphQuadToQuad(q)
	}
	return &world_block_tx.TxSetGraphQuads{Quads: list}
}

// TestReplayWaitsAtAnUnreadableWrite tries to make a replica that cannot read
// a write apply or refuse it, which would part it from the replicas that can.
// A build without the SetGraphQuads body reads a carry write as a WorldOp
// field it does not declare, and a build without a transaction type reads an
// ApplyTxOp of it, alone or in a batch, as an unknown type: each is
// ErrUnreadableOp, on which replay waits for a build that reads it. A build
// that reads the carry write applies it to the World its writer reached.
func TestReplayWaitsAtAnUnreadableWrite(t *testing.T) {
	ctx := t.Context()
	r, base := newWorldReplay(t)
	base = createOn(ctx, t, r, base, "a")
	base = createOn(ctx, t, r, base, "b")
	quads := []world.GraphQuad{
		world.NewGraphQuadWithKeys("a", "<p>", "b", ""),
		world.NewGraphQuadWithKeys("a", "<q>", "b", ""),
		world.NewGraphQuadWithKeys("b", "<p>", "a", ""),
	}

	// A build without the body holds the whole carry write as unknown fields.
	data, err := (&WorldOp{Body: &WorldOp_SetGraphQuads{SetGraphQuads: carryList(quads...)}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	older := &WorldOp{unknownFields: data}
	if reason, err := readUnknownFields(older); reason != "" || !errors.Is(err, ErrUnreadableOp) {
		t.Fatalf("carry write on a build without its body = %q, %v, want ErrUnreadableOp", reason, err)
	}

	// A build that reads it reaches the writer's World: the same list set in
	// one graph update.
	wop := &WorldOp{}
	if err := wop.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if reason, err := readUnknownFields(wop); reason != "" || err != nil {
		t.Fatalf("carry write on this build = %q, %v, want readable", reason, err)
	}
	replayed := carryOn(ctx, t, r, base, quads...)
	written := changeOn(ctx, t, r, base, func(btx *world_block.EngineTx) error {
		return btx.SetGraphQuads(ctx, quads)
	})
	if !replayed.GetHeadRef().GetRootRef().EqualsRef(written.GetHeadRef().GetRootRef()) {
		t.Fatal("the replayed carry write reached another World than its writer")
	}

	// A transaction type this build does not declare waits, alone, batched,
	// or in a batch inside a batch.
	unknown := &world_block_tx.Tx{TxType: 99}
	batch := func(txs ...*world_block_tx.Tx) *world_block_tx.Tx {
		return &world_block_tx.Tx{TxType: world_block_tx.TxType_TxType_BATCH, TxBatch: &world_block_tx.TxBatch{Txs: txs}}
	}
	batched := batch(carryTx(quads...), unknown)
	nested := batch(carryTx(quads...), batch(unknown))
	for _, tx := range []*world_block_tx.Tx{unknown, batched, nested} {
		next, reason, err := r.applyTx(ctx, &ApplyTxOp{Tx: tx}, base, "")
		if next != nil || reason != "" || !errors.Is(err, ErrUnreadableOp) {
			t.Fatalf("unknown transaction type = %v, %q, %v, want ErrUnreadableOp", next, reason, err)
		}
	}
}

// carryTx returns the SET_GRAPH_QUADS transaction that sets quads in order.
func carryTx(quads ...world.GraphQuad) *world_block_tx.Tx {
	return world_block_tx.NewTxSetGraphQuads(carryList(quads...).GetQuads())
}

// faultStore is a store whose puts fail with a full disk from the failAt-th
// put on, counting from one; zero fails none. The stages it opens count and
// fail with it.
type faultStore struct {
	block.StoreOps
	*faults
}

// faults counts a faultStore's puts and sets where they fail.
type faults struct {
	// puts counts the puts.
	puts atomic.Int64
	// failAt is the first put that fails, or zero.
	failAt atomic.Int64
}

// put counts a put and returns its failure.
func (f *faults) put() error {
	n := f.puts.Add(1)
	if at := f.failAt.Load(); at != 0 && n >= at {
		return syscall.ENOSPC
	}
	return nil
}

// PutBlock puts a block unless the disk is full.
func (s *faultStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if err := s.put(); err != nil {
		return nil, false, err
	}
	return s.StoreOps.PutBlock(ctx, data, opts)
}

// PutBlockBatch puts entries unless the disk is full.
func (s *faultStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if err := s.put(); err != nil {
		return nil, err
	}
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

// HoldsRoots reports whether the inner store holds roots.
func (s *faultStore) HoldsRoots() bool {
	return block.HoldsRoots(s.StoreOps)
}

// PinRoot pins ref on the inner store.
func (s *faultStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.StoreOps, ref)
}

// OpenStage opens a stage on the inner store whose puts fail with s's.
func (s *faultStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	stage, release, err := block.OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, nil, err
	}
	return &faultStore{StoreOps: stage, faults: s.faults}, release, nil
}

// ReleaseRoots releases roots on the inner stage.
func (s *faultStore) ReleaseRoots(ctx context.Context, roots []*block.BlockRef) error {
	return s.StoreOps.(block.LeasedRoots).ReleaseRoots(ctx, roots)
}

// TestRegenerateWorldReplicasAgreeThroughFaults runs one regeneration on
// replicas of one World whose stores spill every page. A replica whose disk
// fills partway through the build waits at the operation and refuses
// nothing, then reaches the healthy replica's World once the disk frees. A
// carried quad every replica's World refuses is refused alike on each, so
// replay does not wait at it forever.
func TestRegenerateWorldReplicasAgreeThroughFaults(t *testing.T) {
	ctx := t.Context()
	link := func(subj, pred, obj string) world.GraphQuad {
		return world.NewGraphQuadWithKeys(subj, "<"+pred+">", obj, "")
	}
	conf, err := buildDefaultTransformConf()
	if err != nil {
		t.Fatal(err)
	}

	// Build the same kv World, with the carried quad, on each replica.
	type replica struct {
		r     *spaceReplay
		store *faultStore
		kv    *InnerState
	}
	newReplica := func() *replica {
		t.Helper()
		var fs *faultStore
		r, start := newWorldReplayOn(t, conf, func(inner block.StoreOps) block.StoreOps {
			fs = &faultStore{StoreOps: inner, faults: &faults{}}
			return fs
		}, 1)
		r.a.opts.Regen.SpillDir = t.TempDir()
		kv := createOn(ctx, t, r, start, "a")
		kv = createOn(ctx, t, r, kv, "b")
		kv = changeOn(ctx, t, r, kv, func(btx *world_block.EngineTx) error {
			return btx.SetGraphQuad(ctx, link("a", "p", "b"))
		})
		kv = carryOn(ctx, t, r, kv, link("a", "q", "b"))
		return &replica{r: r, store: fs, kv: kv}
	}
	// forward pins the replica's kv World and switches to the pin, building
	// it inline.
	forward := func(rep *replica) (*InnerState, string, error) {
		target := world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH
		pinned, reason, err := rep.r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: target}, rep.kv)
		if err != nil || reason != "" {
			return nil, reason, err
		}
		pin, _, _ := heldRegen(pinned)
		return rep.r.applyRegenerateWorld(ctx, &RegenerateWorldOp{Target: target, Base: pin}, pinned)
	}

	// The healthy replica regenerates, counting its puts.
	healthy := newReplica()
	before := healthy.store.puts.Load()
	want, reason, err := forward(healthy)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	puts := healthy.store.puts.Load() - before
	if puts < 1 {
		t.Fatalf("the regeneration put %d times through the spilling store", puts)
	}

	// A disk that fills at the build's last put stops replay there; once it
	// frees, the replica reaches the healthy replica's World.
	rep := newReplica()
	rep.store.failAt.Store(rep.store.puts.Load() + puts)
	_, reason, err = forward(rep)
	var gap *regenGapError
	if reason != "" || !errors.As(err, &gap) || !isReplayGap(err) || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("a disk full in the build gave reason %q and error %v, want a replay gap", reason, err)
	}
	rep.store.failAt.Store(0)
	got, reason, err := forward(rep)
	if err != nil || reason != "" {
		t.Fatalf("after a disk full in the build, the retry gave %q, %v", reason, err)
	}
	if !got.EqualVT(want) {
		t.Fatalf("after a disk full in the build, the replica holds %v, want %v", got, want)
	}

	// A carried quad the World refuses, in a run of quad sets the batched
	// path applies, is refused alike on every replica, with no wait.
	var reasons []string
	for _, rep := range []*replica{healthy, newReplica()} {
		bad := []world.GraphQuad{link("a", "r", "b"), world.NewGraphQuad("", "<p>", "<b>", "")}
		_, reason, err := rep.r.applyTx(ctx, &ApplyTxOp{Tx: carryTx(bad...)}, rep.kv, "")
		if err != nil || reasonCode(reason) != "transaction apply failed" {
			t.Fatalf("a quad without a subject gave reason %q and error %v, want a refusal", reason, err)
		}
		reasons = append(reasons, reason)
	}
	if reasons[0] != reasons[1] {
		t.Fatalf("a quad without a subject was refused differently: %q", reasons)
	}
}
