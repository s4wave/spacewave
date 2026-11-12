package space_world

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// floorSpace is a SharedObject whose block store is a bucket, with one
// in-memory local state store and a peer ID.
type floorSpace struct {
	bucketObject
	// local is the local state of every store ID.
	local kvtx.Store
	// peerID is the device's peer ID.
	peerID peer.ID
}

// AccessLocalStateStore returns the in-memory store.
func (s *floorSpace) AccessLocalStateStore(context.Context, string, func()) (kvtx.Store, func(), error) {
	return s.local, func() {}, nil
}

// GetPeerID returns the device's peer ID.
func (s *floorSpace) GetPeerID() peer.ID { return s.peerID }

// floorWorld is a Space World over an in-memory Volume that frees at once,
// whose adapter can be opened again on the same storage, as after a restart.
type floorWorld struct {
	t  *testing.T
	tb *db_testbed.Testbed
	so *floorSpace
	a  *Adapter
	// collect collects the Volume.
	collect func()
}

// newFloorWorld returns a Space World over an in-memory Volume that frees at
// once, with no adapter open.
func newFloorWorld(t *testing.T) *floorWorld {
	// Open the bucket.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: time.Nanosecond.String()},
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

	// Serve it as the Space's block store.
	bkt := cursor.GetBucket()
	roots, ok := bkt.(block.LeasedRoots)
	proofs, pok := bkt.(block.RootProofs)
	if !ok || !pok {
		t.Fatal("the bucket holds no roots or proofs")
	}
	so := &floorSpace{
		bucketObject: bucketObject{store: &bucketStore{StoreOps: bkt, LeasedRoots: roots, RootProofs: proofs}},
		local:        kvtx_inmem.NewStore(),
		peerID:       tb.Volume.GetPeerID(),
	}
	vol := tb.Volume.(*volume_kvtx.Volume)
	collect := func() {
		t.Helper()
		for range 16 {
			res, err := vol.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				return
			}
		}
	}
	return &floorWorld{t: t, tb: tb, so: so, collect: collect}
}

// open opens an adapter keeping at most maxPending bytes of pages, as Execute
// does: it reads the held roots and the saved cursor, then opens the replay
// stage. It ends the earlier adapter's replay first.
func (w *floorWorld) open(maxPending int) {
	w.t.Helper()
	ctx := w.t.Context()
	w.end()
	a, err := NewAdapter(logrus.NewEntry(logrus.New()), w.so, AdapterOptions{
		Bus:             w.tb.Bus,
		StepFactorySet:  w.tb.StepFactorySet,
		MaxPendingBytes: maxPending,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	a.le.Logger.SetLevel(logrus.ErrorLevel)
	a.bucketID = w.tb.BucketId
	a.replay.base = &InnerState{}
	if err := a.loadRoots(ctx); err != nil {
		w.t.Fatal(err)
	}
	if err := a.loadCursor(ctx); err != nil {
		w.t.Fatal(err)
	}
	if a.replay.stage, err = newReplayStage(ctx, a.pages); err != nil {
		w.t.Fatal(err)
	}
	close(a.opened)
	w.a = a
	w.t.Cleanup(w.end)
}

// end waits for the adapter's stores and ends its replay.
func (w *floorWorld) end() {
	if w.a == nil {
		return
	}
	w.a.waitBackground()
	w.a.replay.close()
	w.a.replay.stage.end()
	w.a = nil
}

// op returns the device's ordered operation name with the World operation
// wop.
func (w *floorWorld) op(name string, wop *WorldOp) *sobject.OrderedOperation {
	w.t.Helper()
	data, err := wop.MarshalVT()
	if err != nil {
		w.t.Fatal(err)
	}
	return &sobject.OrderedOperation{
		Hash:  []byte(name),
		Inner: &sobject.SOOperationInner{PeerId: w.so.peerID.String(), LocalId: name},
		Data:  data,
	}
}

// ops returns the operations that initialize a World and then create the
// objects k1 through k<n-1>.
func (w *floorWorld) ops(n int) []*sobject.OrderedOperation {
	w.t.Helper()
	conf, err := buildDefaultTransformConf()
	if err != nil {
		w.t.Fatal(err)
	}
	out := []*sobject.OrderedOperation{w.op("init", &WorldOp{Body: &WorldOp_InitWorld{InitWorld: &InitWorldOp{TransformConf: conf}}})}
	for i := 1; i < n; i++ {
		out = append(out, w.createOp("k"+strconv.Itoa(i)))
	}
	return out
}

// createOp returns the operation that creates the object key.
func (w *floorWorld) createOp(key string) *sobject.OrderedOperation {
	return w.op(key, &WorldOp{Body: &WorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{Tx: &world_block_tx.Tx{
		TxType:         world_block_tx.TxType_TxType_CREATE_OBJECT,
		TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: key},
	}}}})
}

// apply applies op at index and fails on an error or a refusal.
func (w *floorWorld) apply(index int, op *sobject.OrderedOperation) {
	w.t.Helper()
	reason, err := w.a.Apply(w.t.Context(), index, op)
	if err != nil || reason != "" {
		w.t.Fatalf("apply %s at %d: %q %v", op.Hash, index, reason, err)
	}
}

// root stores the World after n positions under the checkpoint root, as the
// signer's checkpoint does, and returns it.
func (w *floorWorld) root(n int) *InnerState {
	w.t.Helper()
	ctx := w.t.Context()
	data, store, err := w.a.Root(ctx, n)
	if err != nil {
		w.t.Fatalf("root %d: %v", n, err)
	}
	if err := store(ctx); err != nil {
		w.t.Fatalf("store the World after %d positions: %v", n, err)
	}
	state := &InnerState{}
	if err := state.UnmarshalVT(data); err != nil {
		w.t.Fatal(err)
	}
	return state
}

// requireKeys fails unless the World state holds exactly the objects k1
// through k<n-1> and extra.
func (w *floorWorld) requireKeys(what string, state *InnerState, n int, extra ...string) {
	w.t.Helper()
	keys := []string{"k" + strconv.Itoa(n)}
	for i := 1; i < n; i++ {
		keys = append(keys, "k"+strconv.Itoa(i))
	}
	keys = append(keys, extra...)
	got := hasKeys(w.t.Context(), w.t, w.a.replay, state, keys...)
	for i, ok := range got {
		if ok != (i != 0) {
			w.t.Fatalf("%s holds %v = %v", what, keys, got)
		}
	}
}

// TestRootRecomputesThroughTheFloor checks that a checkpoint's Root below the
// replay floor, after a full save swept the earlier Worlds' pages or after a
// restart restored the positions from the saved cursor, leaves no position
// below the floor read as computed: a second Root inside that range stores a
// readable World, and a reorder applied inside it runs on a readable World.
// It fails when recompute stops at the asked position and clears the floor.
func TestRootRecomputesThroughTheFloor(t *testing.T) {
	const positions = 7
	check := func(t *testing.T, w *floorWorld) {
		// The positions from 2 to the floor have Worlds that left memory.
		if w.a.replay.floor < 5 {
			t.Fatalf("the floor is %d, want at least 5", w.a.replay.floor)
		}
		w.collect()

		// A checkpoint below the floor, then one inside the range it left.
		w.requireKeys("the World after 2 positions", w.root(2), 2)
		w.collect()
		w.requireKeys("the World after 4 positions", w.root(4), 4)
		w.collect()

		// A reorder inside the range runs on the World before it.
		w.apply(3, w.createOp("x"))
		state, _ := w.a.replay.head()
		w.requireKeys("the reordered head", state, 3, "x")
	}

	t.Run("full save", func(t *testing.T) {
		// Keep the positions' pages, then fill the pages so the last position
		// saves the cursor and sweeps the earlier Worlds' pages.
		w := newFloorWorld(t)
		w.open(1 << 20)
		ops := w.ops(positions)
		for i, op := range ops[:positions-1] {
			w.apply(i, op)
		}
		filler := make([]byte, 1<<19)
		ref, err := block.BuildBlockRef(filler, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.a.pages.PutBlockBatch(t.Context(), []*block.PutBatchEntry{{Ref: ref, Data: filler, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		w.apply(positions-1, ops[positions-1])
		w.a.waitBackground()
		check(t, w)
	})

	t.Run("restore", func(t *testing.T) {
		// Save the cursor, then open again and replay the same order.
		w := newFloorWorld(t)
		w.open(1 << 20)
		ops := w.ops(positions)
		for i, op := range ops {
			w.apply(i, op)
		}
		if err := w.a.saveCursor(t.Context()); err != nil {
			t.Fatal(err)
		}
		w.open(1 << 20)
		for i, op := range ops {
			w.apply(i, op)
		}
		check(t, w)
	})
}
