package sobject_world_engine

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// hidingBlockStore withholds one block while hidden is set, as a store does
// when no peer supplies the block.
type hidingBlockStore struct {
	bstore.BlockStore
	ref    *block.BlockRef
	hidden atomic.Bool
	// err is the error a read of the hidden block returns, or nil to report
	// the block missing.
	err error
	// peer, when set, makes the hidden block one only an absent peer holds:
	// a read that may wait for a peer signals asked and blocks until peer is
	// closed, and a read that may not records the block and reports it
	// unavailable.
	peer  chan struct{}
	asked chan struct{}
}

// GetBlock reads a block, withholding the hidden block.
func (s *hidingBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.hidden.Load() && ref.EqualsRef(s.ref) {
		if s.peer == nil {
			return nil, false, s.err
		}
		if peerWait := block.GetPeerWait(ctx); peerWait != nil {
			peerWait.Skip(ref)
			return nil, false, block.ErrUnavailable
		}
		select {
		case s.asked <- struct{}{}:
		default:
		}
		select {
		case <-s.peer:
		case <-ctx.Done():
			return nil, false, context.Cause(ctx)
		}
	}
	return s.BlockStore.GetBlock(ctx, ref)
}

// serve supplies the hidden block from now on.
func (s *hidingBlockStore) serve() {
	if s.hidden.Swap(false) && s.peer != nil {
		close(s.peer)
	}
}

// TestReplayRetriesCanceledBlockRead keeps the replay context live while a
// block read's transport lifetime ends. The failed operation stays unplaced,
// including in the saved cursor, and applies after the transport is replaced.
func TestReplayRetriesCanceledBlockRead(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			// Build a real World that resolves the object edit operation.
			ctx := t.Context()
			priv, pid := newReplayTestKey(t)
			space := newReplayTestSpace(t, pid)
			space.c.SetOpTypes(OpTypes{Lookup: world_mock.LookupMockOp})

			// Write the encrypted payload the edit will read.
			conf := space.genesis.GetHeadRef().GetTransformConf()
			xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, space.c.sfs, conf)
			if err != nil {
				t.Fatal(err)
			}
			btx, bcs := block.NewTransaction(space.so.blockStore, xfrm, nil, nil)
			bcs.SetBlock(&block_mock.Example{Msg: "before"}, true)
			payload, _, err := btx.Write(ctx, true)
			if err != nil {
				t.Fatal(err)
			}

			// Sign an object creation followed by an edit that reads its payload.
			create, err := world_block_tx.NewTxCreateObject("object", &bucket.ObjectRef{RootRef: payload, TransformConf: conf})
			if err != nil {
				t.Fatal(err)
			}
			edit, err := world_block_tx.NewTxApplyObjectOp(world_mock.MockObjectOpTypeID, world_mock.NewMockObjectOp("after"), "object", pid)
			if err != nil {
				t.Fatal(err)
			}
			o1 := space.signTx("create", priv, create, &sobject.SOOperationLink{Nonce: 1})
			o2 := space.signTx("edit", priv, edit, &sobject.SOOperationLink{Nonce: 2, PrevOpHash: o1.Hash()})

			// Deliver the signed operations to the replaying member.
			member := space.member(pid, false)
			member.replayer.lookupOp = space.c.buildLookupWorldOp(space.c.le)
			for _, op := range []*sobject.SOOperation{o1, o2} {
				if _, err := member.set.Add(op); err != nil {
					t.Fatal(err)
				}
			}

			// End the child read lifetime without ending the replay's context.
			readCtx, cancel := context.WithCancelCause(ctx)
			cancel(want)
			store := &hidingBlockStore{BlockStore: space.so.blockStore, ref: payload, err: errors.Wrap(context.Cause(readCtx), "transport read")}
			store.hidden.Store(true)
			member.replayer.so = &testSharedObject{peerID: pid, blockStore: store}
			_, _, err = member.replayer.replay(ctx, member.snap, member.set, nil)
			if !errors.Is(err, want) || ctx.Err() != nil {
				t.Fatalf("replay error=%v context=%v, want child error %v", err, ctx.Err(), want)
			}
			if got := len(member.replayer.positions); got != 1 {
				t.Fatalf("replay placed %d operations, want only the creation", got)
			}

			// A saved cursor must not turn the interruption into a rejection.
			data, err := member.replayer.encodeCursor()
			if err != nil {
				t.Fatal(err)
			}
			cursor := &ReplayCursor{}
			if err := cursor.UnmarshalVT(data); err != nil {
				t.Fatal(err)
			}
			if len(cursor.GetOutcomes()) != 1 || cursor.GetOutcomes()[0].GetReason() != "" {
				t.Fatalf("saved replay contains an interrupted operation: %v", cursor.GetOutcomes())
			}

			// Replacing the transport lets the same edit apply successfully.
			store.serve()
			state, outcomes, err := member.replayer.replay(ctx, member.snap, member.set, nil)
			if err != nil || len(outcomes) != 2 || outcomes[1].reason != "" {
				t.Fatalf("resumed replay outcomes=%v error=%v", outcomes, err)
			}

			// Bind the resumed World for a fresh read.
			w := newReplayWorld(space.c, store)
			defer w.close(ctx)
			ws, err := w.bind(ctx, state.GetHeadRef())
			if err != nil {
				t.Fatal(err)
			}

			// Read the edited payload through the object API.
			tx, err := ws.bengine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			obj, found, err := tx.GetObject(ctx, "object")
			defer world.ReleaseObjectState(obj)
			if err != nil || !found {
				t.Fatalf("resumed object found=%v error=%v", found, err)
			}
			_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
				value, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
				if err == nil && value.(*block_mock.Example).GetMsg() != "after" {
					t.Fatalf("resumed edit left payload %q", value.(*block_mock.Example).GetMsg())
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// opCounter counts the operations replay starts to apply.
type opCounter struct {
	n atomic.Int32
}

// Levels returns the level processOp logs a starting operation at.
func (c *opCounter) Levels() []logrus.Level {
	return []logrus.Level{logrus.DebugLevel}
}

// Fire counts a starting operation.
func (c *opCounter) Fire(entry *logrus.Entry) error {
	if entry.Message == "processing op" {
		c.n.Add(1)
	}
	return nil
}

// TestEngineStopsReplayAtMissingBlock replays a log whose last operation needs
// a block that is not available, because the store lacks it, because no peer
// served it, or because only an absent peer holds it. The initial replay and
// each later one must stop at that operation without holding the write lock,
// serve the World before it, fail writers with the read error, and resume at
// that operation, never applying an earlier operation again, once the block
// arrives. A writer waits for a block only an absent peer holds, without
// holding the write lock, and the block must also wake the watcher when it
// arrives.
func TestEngineStopsReplayAtMissingBlock(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		testEngineStopsReplayAtBlockGap(t, &hidingBlockStore{}, block.ErrNotFound)
	})
	t.Run("unserved", func(t *testing.T) {
		unserved := errors.Wrap(block.ErrUnavailable, "no peer served block")
		testEngineStopsReplayAtBlockGap(t, &hidingBlockStore{err: unserved}, block.ErrUnavailable)
	})
	t.Run("absent-peer", func(t *testing.T) {
		store := &hidingBlockStore{peer: make(chan struct{}), asked: make(chan struct{}, 1)}
		testEngineStopsReplayAtBlockGap(t, store, block.ErrUnavailable)
	})
}

// testEngineStopsReplayAtBlockGap runs TestEngineStopsReplayAtMissingBlock
// with store hiding the block. Writers must fail with an error matching want.
func testEngineStopsReplayAtBlockGap(t *testing.T, store *hidingBlockStore, want error) {
	// Build a Space whose operations create three objects, create one whose
	// root block is missing, and then edit it.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	space.c.SetOpTypes(OpTypes{Lookup: world_mock.LookupMockOp})

	// Write the block the fourth operation's object uses as its root.
	transformConf := space.genesis.GetHeadRef().GetTransformConf()
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, space.c.sfs, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := block.NewTransaction(space.so.blockStore, xfrm, nil, nil)
	bcs.SetBlock(&block_mock.Example{Msg: "payload"}, true)
	payload, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the fourth operation's object on that root and the fifth's edit
	// of it.
	ghost, err := world_block_tx.NewTxCreateObject("ghost", &bucket.ObjectRef{RootRef: payload, TransformConf: transformConf})
	if err != nil {
		t.Fatal(err.Error())
	}
	edit, err := world_block_tx.NewTxApplyObjectOp(world_mock.MockObjectOpTypeID, world_mock.NewMockObjectOp("edited"), "ghost", pidA)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Sign the five operations as one chain.
	o1 := space.sign("o1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	o2 := space.sign("o2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: o1.Hash()})
	o3 := space.sign("o3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: o2.Hash()})
	o4 := space.signTx("ghost", privA, ghost, &sobject.SOOperationLink{Nonce: 4, PrevOpHash: o3.Hash()})
	o5 := space.signTx("edit", privA, edit, &sobject.SOOperationLink{Nonce: 5, PrevOpHash: o4.Hash()})

	// Encode the genesis World as the checkpoint state.
	genesisData, err := space.genesis.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	checkpoint := &sobject.SOCheckpointInner{StateData: genesisData}

	// Order the operations above the checkpoint.
	set := sobject.NewSOOperationSet(replayTestObjectID, checkpoint)
	for _, op := range []*sobject.SOOperation{o1, o2, o3, o4, o5} {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Build the engine over a store that hides the root block.
	snap := &replayTestSnapshot{config: space.config, checkpoint: checkpoint, set: set}
	if snap.participant, err = snap.GetParticipantConfigForPeer(ctx, pidA.String()); err != nil {
		t.Fatal(err.Error())
	}
	store.BlockStore, store.ref = space.so.blockStore, payload
	store.hidden.Store(true)
	so := &testSharedObject{peerID: pidA, blockStore: store, localStore: store_kvtx_inmem.NewStore(), snapshot: snap}

	// The initial replay serves the World before the missing block.
	stateCtr := ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](snap)
	if _, err := space.c.waitWorldInit(ctx, so, stateCtr, newReplayer(space.c, so)); err != nil {
		t.Fatalf("initial replay with a missing block = %v; want the World before it", err)
	}

	// Build the engine over the same store.
	blk, err := buildBlockEngine(ctx, space.c.le, space.c.bus, space.c.sfs, so.GetBlockStore(), space.genesis.GetHeadRef(), transformConf, space.c.buildLookupWorldOp(space.c.le), false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(blk.Release)
	engine := newSoEngine(ctx, space.c, so, blk, newReplayer(space.c, so))

	// Count the operations replay applies.
	applied := &opCounter{}
	space.c.le.Logger.AddHook(applied)
	space.c.le.Logger.SetLevel(logrus.DebugLevel)
	space.c.le.Logger.SetOutput(io.Discard)
	wantApplied := func(want int32) {
		t.Helper()
		if got := applied.n.Load(); got != want {
			t.Fatalf("replay started %d operations; want %d", got, want)
		}
	}

	// writeLockFree fails the test when the write lock stays held.
	writeLockFree := func() {
		// Take and drop the lock within two seconds.
		t.Helper()
		lockCtx, lockCancel := context.WithTimeout(ctx, 2*time.Second)
		defer lockCancel()
		unlock, err := space.c.writeMtx.Lock(lockCtx, "test")
		if err != nil {
			t.Fatalf("write lock is held: %v", err)
		}
		unlock()
	}

	// writeStops opens a writer while the block is missing. The writer fails
	// with the read error, or, when only an absent peer holds the block, waits
	// for the peer without holding the write lock until its context ends.
	writeStops := func(name string) {
		// Expect the read error when no peer will serve the block.
		t.Helper()
		if store.peer == nil {
			if tx, err := engine.NewTransaction(ctx, true); !errors.Is(err, want) {
				if err == nil {
					tx.Discard()
				}
				t.Fatalf("%s with a missing block = %v; want %v", name, err, want)
			} else if op := world_mock.MockObjectOpTypeID + " on ghost"; !strings.Contains(err.Error(), op) {
				t.Fatalf("%s with a missing block = %v; want it to name %q", name, err, op)
			}
			return
		}

		// Otherwise start the writer, dropping any earlier wait signal.
		select {
		case <-store.asked:
		default:
		}
		writeCtx, writeCancel := context.WithCancel(ctx)
		defer writeCancel()
		done := make(chan error, 1)
		go func() {
			tx, err := engine.NewTransaction(writeCtx, true)
			if err == nil {
				tx.Discard()
			}
			done <- err
		}()

		// Cancel it once it waits for the peer with the write lock free.
		select {
		case <-store.asked:
		case err := <-done:
			t.Fatalf("%s ended before waiting for an absent peer: %v", name, err)
		}
		writeLockFree()
		writeCancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("%s waiting for an absent peer = %v; want it canceled", name, err)
		}
	}

	// A writer stops at the missing block after replay applied the four
	// operations before it and tried the fifth.
	writeStops("write")
	wantApplied(5)
	writeLockFree()

	// Readers see the World before the missing block.
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, key := range []string{"object-1", "object-2", "object-3", "ghost"} {
		objectState, found, err := read.GetObject(ctx, key)
		world.ReleaseObjectState(objectState)
		if err != nil || !found {
			t.Fatalf("object %s before the missing block: found %v, %v", key, found, err)
		}
	}
	read.Discard()

	// The watcher keeps running through a state change and retries only the
	// operation that failed, as does the next writer.
	if err := space.c.executeWatchSOStateOnce(ctx, snap, engine); err != nil {
		t.Fatalf("watcher ended on a missing block: %v", err)
	}
	wantApplied(6)
	writeStops("second write")
	wantApplied(7)
	writeLockFree()

	// A block only an absent peer holds wakes the watcher when it arrives.
	if store.peer != nil {
		if !engine.replay.gap.missing.EqualsRef(payload) {
			t.Fatalf("replay stopped at %v; want %s", engine.replay.gap.missing, payload.MarshalString())
		}
		woke := make(chan error, 1)
		go func() {
			_, err := space.c.waitReplayInput(ctx, so, stateCtr, snap, engine.replay.gap, nil)
			woke <- err
		}()
		store.serve()
		if err := <-woke; err != nil {
			t.Fatalf("watcher wait after the block arrived: %v", err)
		}
	}

	// Once the block arrives, a writer resumes at the operation that stopped.
	store.serve()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("write after the block arrived: %v", err)
	}
	tx.Discard()
	wantApplied(8)
}
