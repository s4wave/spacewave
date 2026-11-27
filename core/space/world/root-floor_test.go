package space_world

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
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
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
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
	a.replay.stage = newReplayStage(a.pages)
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

// TestCursorRefusalRulePreservesPositions loads each rule twice without a save.
// Only hashes of refusals an old rule may have made about the device change:
// any refusal under REFUSES_MISSING_HANDLER, and an unknown operation type
// under WAITS_FOR_HANDLER. The stored cursor stays byte-identical.
func TestCursorRefusalRulePreservesPositions(t *testing.T) {
	for _, rule := range []ReplayRefusalRule{
		ReplayRefusalRule_REPLAY_REFUSAL_RULE_REFUSES_MISSING_HANDLER,
		ReplayRefusalRule_REPLAY_REFUSAL_RULE_WAITS_FOR_HANDLER,
		ReplayRefusalRule_REPLAY_REFUSAL_RULE_WAITS_FOR_UPGRADE,
	} {
		t.Run(rule.String(), func(t *testing.T) {
			// Save applied and refused positions over real in-memory storage.
			ctx := t.Context()
			w := newFloorWorld(t)
			w.open(1 << 20)
			ops := w.ops(3)

			// Persist a successful prefix followed by a refusal.
			w.apply(0, ops[0])
			w.apply(1, ops[1])
			ops[2].Refused = "unknown operation type"
			if reason, err := w.a.Apply(ctx, 2, ops[2]); err != nil || reason == "" {
				t.Fatalf("historical refusal = %q, %v", reason, err)
			}
			if err := w.a.saveCursor(ctx); err != nil {
				t.Fatal(err)
			}
			w.a.waitBackground()

			// Select the historical rule without changing any saved outcome.
			cursor, err := readSavedCursor(ctx, w.so)
			if err != nil {
				t.Fatal(err)
			}
			if cursor.GetRefusalRule() != ReplayRefusalRule_REPLAY_REFUSAL_RULE_WAITS_FOR_UPGRADE {
				t.Fatal("a newly encoded cursor did not record the current rule")
			}

			// Retain the same outcomes under the selected rule.
			cursor.RefusalRule = rule
			data, err := cursor.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			if err := writeLocalState(ctx, w.so, replayCursorStoreID, replayCursorKey, data); err != nil {
				t.Fatal(err)
			}
			states, ok := cursor.states()
			if !ok {
				t.Fatal("fixture cursor does not reach its head")
			}

			// Loading twice must preserve every position and the saved record.
			w.open(1 << 20)
			for range 2 {
				if err := w.a.loadCursor(ctx); err != nil {
					t.Fatal(err)
				}
				if len(w.a.replay.restored) != len(cursor.GetOutcomes()) {
					t.Fatal("loading dropped a cursor position")
				}
				for i, pos := range w.a.replay.restored {
					outcome := cursor.GetOutcomes()[i]
					hash := outcome.GetHash()
					if rule != ReplayRefusalRule_REPLAY_REFUSAL_RULE_WAITS_FOR_UPGRADE && outcome.GetReason() != "" {
						hash = nil
					}
					if !bytes.Equal(pos.op.Hash, hash) || pos.reason != outcome.GetReason() || pos.seqno != outcome.GetSeqno() || !pos.state.EqualVT(states[i]) || !slices.EqualFunc(pos.payloads, outcome.GetPayloads(), (*block.BlockRef).EqualVT) || !pos.handlers.EqualVT(outcome.GetPayloadHandlers()) {
						t.Fatalf("load changed position %d beyond its old refused hash", i)
					}
				}
				stored, found, err := readLocalState(ctx, w.so, replayCursorStoreID, replayCursorKey)
				if err != nil || !found || !bytes.Equal(stored, data) {
					t.Fatalf("load rewrote the saved cursor: found %v, error %v", found, err)
				}
				if !w.a.replay.restoredFrom.EqualVT(cursor.GetBase()) {
					t.Fatal("load changed the cursor's base")
				}
			}
		})
	}
}

// TestCursorRerunSurvivesRestart replays an old cursor through a missing
// handler. A changed World invalidates successful successors. The
// non-idempotent revision increment must survive exactly once beside member and
// deterministic refusals.
func TestCursorRerunSurvivesRestart(t *testing.T) {
	// Build a transaction that creates an object and needs the plugin handler.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	w := newFloorWorld(t)
	w.open(1 << 20)

	// Encode both refused operations through the same plugin transaction type.
	pluginOp := func(key string) *sobject.OrderedOperation {
		t.Helper()
		tx, err := world_block_tx.NewTxApplyWorldOp(world_mock.NewMockWorldOp(key, "recovered"), w.so.peerID)
		if err != nil {
			t.Fatal(err)
		}
		return w.op(key, &WorldOp{Body: &WorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{Tx: &world_block_tx.Tx{
			TxType: world_block_tx.TxType_TxType_BATCH,
			TxBatch: &world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{
				{TxType: world_block_tx.TxType_TxType_CREATE_OBJECT, TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: key}},
				tx,
			}},
		}}}})
	}
	ops := append(w.ops(2), pluginOp("member"), pluginOp("recovered"), w.createOp("after"))
	ops[2].Refused = "member verdict"
	ops = append(ops, w.op("increment", &WorldOp{Body: &WorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{Tx: &world_block_tx.Tx{
		TxType:         world_block_tx.TxType_TxType_OBJECT_INC_REV,
		TxObjectIncRev: &world_block_tx.TxObjectIncRev{ObjectKey: "k1"},
	}}}}), w.op("duplicate", &WorldOp{Body: &WorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{Tx: &world_block_tx.Tx{
		TxType:         world_block_tx.TxType_TxType_CREATE_OBJECT,
		TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: "k1"},
	}}}}))

	// Save the old build's view, where only the handler operation was skipped.
	for i, op := range ops {
		historical := *op
		if i == 3 {
			historical.Refused = "an opaque old verdict"
		}
		reason, err := w.a.Apply(ctx, i, &historical)
		if err != nil || (reason != "") != (i == 2 || i == 3 || i == 6) {
			t.Fatalf("historical position %d = %q, %v", i, reason, err)
		}
	}
	if err := w.a.saveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	w.a.waitBackground()

	// Store the historical rule while keeping the saved Worlds and payloads.
	old, err := readSavedCursor(ctx, w.so)
	if err != nil {
		t.Fatal(err)
	}
	old.RefusalRule = ReplayRefusalRule_REPLAY_REFUSAL_RULE_REFUSES_MISSING_HANDLER
	data, err := old.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLocalState(ctx, w.so, replayCursorStoreID, replayCursorKey, data); err != nil {
		t.Fatal(err)
	}

	// Restart after clearing in memory but before replay or a current-rule save.
	w.open(1 << 20)
	if err := w.a.loadCursor(ctx); err != nil {
		t.Fatal(err)
	}
	w.open(1 << 20)
	w.a.runCtx = ctx
	defer func() { w.a.endHandlerWait() }()

	// Preserve the member verdict and stop at the adjacent missing handler.
	for i, op := range ops[:3] {
		reason, err := w.a.Apply(ctx, i, op)
		if err != nil || (reason != "") != (i == 2) {
			t.Fatalf("replay prefix %d = %q, %v", i, reason, err)
		}
	}
	reason, err := w.a.Apply(ctx, 3, ops[3])
	var wait *sobject.BodyWait
	if reason != "" || !errors.As(err, &wait) {
		t.Fatalf("missing handler = %q, %v, want a typed wait", reason, err)
	}
	if len(w.a.replay.positions) != 3 {
		t.Fatal("waiting recorded an outcome for the missing handler")
	}

	// A wait must leave the old cursor available to another restart.
	stored, err := readSavedCursor(ctx, w.so)
	if err != nil || !stored.EqualVT(old) {
		t.Fatalf("restart or wait changed the saved cursor: %v", err)
	}

	// Register the real handler, take its wake, and replay the changed suffix.
	release, err := w.tb.Bus.AddController(ctx, world.NewLookupOpController("test-plugin", "", world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	select {
	case <-wait.Wake:
	case <-ctx.Done():
		t.Fatal("handler registration never woke replay")
	}

	// Recompute the successors on the World the recovered operation changed.
	for i := 3; i < len(ops); i++ {
		reason, err := w.a.Apply(ctx, i, ops[i])
		if err != nil || (reason != "") != (i == 6) {
			t.Fatalf("recovered position %d = %q, %v", i, reason, err)
		}
	}
	head, seqno := w.a.replay.head()
	if got := hasKeys(ctx, t, w.a.replay, head, "k1", "recovered", "after", "member"); !slices.Equal(got, []bool{true, true, true, false}) {
		t.Fatalf("recovered World membership = %v", got)
	}

	// Read the revision to detect a lost or duplicated increment.
	engine, err := w.a.replay.bind(ctx, head.GetHeadRef())
	if err != nil {
		t.Fatal(err)
	}
	err = world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		// Release the object on every path after acquiring it.
		obj, err := world.MustGetObject(ctx, ws, "k1")
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)

		// Exactly one increment follows the object's creation.
		_, rev, err := obj.GetRootRef(ctx)
		if err == nil && rev != 2 {
			t.Fatalf("revision increment applied %d times, want once", rev-1)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Restart once before saving and once after saving the current rule.
	w.a.endHandlerWait()
	for _, save := range []bool{false, true} {
		if save {
			if err := w.a.saveCursor(ctx); err != nil {
				t.Fatal(err)
			}
			w.a.waitBackground()
			current, err := readSavedCursor(ctx, w.so)
			if err != nil || current.GetRefusalRule() != ReplayRefusalRule_REPLAY_REFUSAL_RULE_WAITS_FOR_UPGRADE {
				t.Fatalf("current-rule save = %v, %v", current, err)
			}
		}
		w.open(1 << 20)
		for i, op := range ops {
			reason, err := w.a.Apply(ctx, i, op)
			if err != nil || (reason != "") != (i == 2 || i == 6) {
				t.Fatalf("restart position %d = %q, %v", i, reason, err)
			}
		}
		got, gotSeqno := w.a.replay.head()
		if !got.EqualVT(head) || gotSeqno != seqno {
			t.Fatalf("restart after save %v lost, duplicated or invented an effect: seqno %d, want %d", save, gotSeqno, seqno)
		}
		w.collect()
		if got := hasKeys(ctx, t, w.a.replay, got, "k1", "recovered", "after", "member"); !slices.Equal(got, []bool{true, true, true, false}) {
			t.Fatalf("collection after save %v changed World membership: %v", save, got)
		}
	}

	// Die between the replay's overflow puts and the cursor save. Under a
	// one-byte bound every page goes to the epoch's stage and every
	// position starts a save, but a save already running skips those
	// positions, and ending the adapter then ends the stage unsaved, as a
	// lapsed lease does.
	all := append(slices.Clone(ops), w.createOp("late1"), w.createOp("late2"))
	keys := []string{"k1", "recovered", "after", "member", "late1", "late2", "late3"}
	replayAll := func(phase string) *InnerState {
		t.Helper()
		for i, op := range all {
			reason, err := w.a.Apply(ctx, i, op)
			if err != nil || (reason != "") != (i == 2 || i == 6) {
				t.Fatalf("%s position %d = %q, %v", phase, i, reason, err)
			}
		}
		w.a.waitBackground()
		got, _ := w.a.replay.head()
		return got
	}
	requireKeys := func(phase string, state *InnerState) {
		t.Helper()
		want := []bool{true, true, true, false, true, true, len(all) > 9}
		if got := hasKeys(ctx, t, w.a.replay, state, keys...); !slices.Equal(got, want) {
			t.Fatalf("World %s = %v", phase, got)
		}
	}
	gated := &gatedRoots{bucketStore: w.so.store.(*bucketStore)}
	w.so.store = gated
	w.open(1)
	w.a.cursorSaving.Store(true)
	replayAll("unsaved")
	if !w.a.replay.stage.epoch.Stored() || w.a.replay.saved == len(all) {
		t.Fatal("the replayed pages did not overflow to an unsaved epoch")
	}
	w.open(1)
	w.collect()
	requireKeys("after the death", replayAll("after the death"))

	// Collect while a save's root move waits: the epoch the save rotated out
	// still holds the pages of the World it saves.
	w.a.cursorSaving.Store(true)
	w.apply(len(all), w.createOp("late3"))
	all = append(all, w.createOp("late3"))
	w.a.cursorSaving.Store(false)
	if !w.a.replay.stage.epoch.Stored() {
		t.Fatal("the position's pages did not overflow to its epoch")
	}
	gate := make(chan struct{})
	gated.gate.Store(&gate)
	if err := w.a.saveCursor(ctx); err != nil {
		t.Fatal(err)
	}
	w.collect()
	gated.gate.Store(nil)
	close(gate)
	w.a.waitBackground()
	head, _ = w.a.replay.head()
	w.open(1)
	w.collect()
	if got := replayAll("after the saved overflow"); !got.EqualVT(head) {
		t.Fatal("restart after the saved overflow changed the head World")
	}
	requireKeys("after the saved overflow", head)
}

// gatedRoots is a bucket whose root moves wait on gate while it is set.
type gatedRoots struct {
	*bucketStore
	// gate is closed to let the waiting root moves through.
	gate atomic.Pointer[chan struct{}]
}

// SetRoots waits on the gate, then moves the roots.
func (s *gatedRoots) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if gate := s.gate.Load(); gate != nil {
		select {
		case <-*gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.bucketStore.SetRoots(ctx, entries, roots)
}
