package world_block

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	kvtx_cayley "github.com/aperturerobotics/hydra/kvtx/cayley"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	world_vlogger "github.com/aperturerobotics/hydra/world/vlogger"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	cayley_kv "github.com/cayleygraph/cayley/graph/kv"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// WorldState implements world state backed by a block graph.
// Note: calls are not concurrency safe. Use Tx if you want a mutex.
type WorldState struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	le        *logrus.Entry
	btx       *block.Transaction
	bcs       *block.Cursor
	write     bool
	verbose   bool

	objTree   kvtx.BlockTx
	graphTree kvtx.BlockTx
	graphHd   *cayley.Handle

	storage  world.WorldStorage
	lookupOp world.LookupOp

	pendingChanges []*block.Cursor // *WorldChange

	// mtx guards below fields
	mtx sync.Mutex
	// waiters are callbacks that should be called when seqno changes
	waiters []func(seqno uint64)
}

// NewWorldState constructs a new world handle.
// btx can be nil to not write during Commit()
// bcs is located at the root of the world (the World block).
// if bcs is empty, creates a new empty world.
// world and object op handlers manage applying batch operations.
func NewWorldState(
	ctx context.Context,
	le *logrus.Entry,
	write bool,
	btx *block.Transaction,
	bcs *block.Cursor,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
) (*WorldState, error) {
	tx := &WorldState{
		btx:   btx,
		bcs:   bcs,
		le:    le,
		write: write,

		storage:  storage,
		lookupOp: lookupOp,
	}
	tx.ctx, tx.ctxCancel = context.WithCancel(ctx)
	if err := tx.SetBlockTransaction(btx, bcs); err != nil {
		return nil, err
	}
	return tx, nil
}

// BuildWorldStateFromCursor builds a world state from a bucket lookup cursor.
func BuildWorldStateFromCursor(
	ctx context.Context,
	le *logrus.Entry,
	write bool,
	bls *bucket_lookup.Cursor,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
) (*WorldState, error) {
	btx, bcs := bls.BuildTransaction(nil)
	return NewWorldState(ctx, le, write, btx, bcs, storage, lookupOp)
}

// SetVerbose sets if the ApplyWorldOp calls should log verbosely.
func (t *WorldState) SetVerbose(verbose bool) {
	t.verbose = verbose
}

// GetReadOnly returns if the world handle is read-only.
func (t *WorldState) GetReadOnly() bool {
	return !t.write
}

// GetRootRef returns the current root reference.
func (t *WorldState) GetRootRef() *block.BlockRef {
	return t.bcs.GetRef()
}

// GetBcs returns the root block cursor.
func (t *WorldState) GetBcs() *block.Cursor {
	return t.bcs
}

// GetRoot builds the Root object from the block cursor.
func (t *WorldState) GetRoot() (*World, error) {
	return UnmarshalWorld(t.bcs)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (t *WorldState) GetSeqno() (uint64, error) {
	w, err := t.GetRoot()
	if err != nil {
		return 0, err
	}
	return w.GetLastChange().GetSeqno(), nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (t *WorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	if t.GetReadOnly() {
		// read-only txns cannot change seqno, waiting doesn't make sense.
		return 0, tx.ErrNotWrite
	}

	for {
		t.mtx.Lock()
		w, err := t.GetRoot()
		if err != nil {
			t.mtx.Unlock()
			return 0, err
		}
		seqno := w.GetLastChange().GetSeqno()
		var waitCh chan uint64
		tooOld := seqno < value
		if err == nil && tooOld {
			waitCh = make(chan uint64, 1)
			t.waiters = append(t.waiters, func(seqno uint64) {
				select {
				case waitCh <- seqno:
				default:
				}
			})
		}
		t.mtx.Unlock()
		if err != nil {
			return 0, err
		}
		if !tooOld {
			return seqno, nil
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case seqno = <-waitCh:
			// seqno updated
			if seqno >= value {
				return seqno, nil
			}
		}
	}
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (t *WorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	storage := t.storage
	if storage == nil {
		return nil, world.ErrWorldStorageUnavailable
	}
	return storage.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *WorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	storage := t.storage
	if storage == nil {
		return world.ErrWorldStorageUnavailable
	}
	return storage.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (t *WorldState) ApplyWorldOp(
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}

	if err := op.Validate(); err != nil {
		return 0, false, err
	}

	subCtx, subCtxCancel := context.WithCancel(t.ctx)
	defer subCtxCancel()

	var ws world.WorldState = t
	if t.verbose {
		ws = world_vlogger.NewWorldState(
			t.le.WithField("apply-op-type", op.GetOperationTypeId()),
			ws,
		)
	}

	sysErr, err := op.ApplyWorldOp(subCtx, t.le, ws, opSender)
	if err != nil {
		return 0, sysErr, err
	}
	seq, err := t.GetSeqno()
	if err != nil {
		return 0, true, err
	}
	seq += uint64(len(t.pendingChanges))
	return seq, false, nil
}

// Fork forks the current world state into a completely separate world state.
//
// Creates a new block transaction.
func (t *WorldState) Fork(ctx context.Context) (world.WorldState, error) {
	bcs := t.bcs.DetachTransaction()
	blk, _ := bcs.GetBlock()
	var blkv *World
	if blk != nil {
		var ok bool
		blkv, ok = blk.(*World)
		if !ok {
			return nil, block.ErrUnexpectedType
		}
	}
	if blkv != nil {
		blkv = proto.Clone(blkv).(*World)
		bcs.SetBlock(blkv, false)
	} else {
		blkv = &World{}
		bcs.SetBlock(blkv, true)
	}
	return NewWorldState(
		ctx,
		t.le,
		t.write,
		bcs.GetTransaction(),
		bcs,
		t.storage,
		t.lookupOp,
	)
}

// SetBlockTransaction loads the state from the given block transaction and cursor.
func (t *WorldState) SetBlockTransaction(btx *block.Transaction, bcs *block.Cursor) error {
	// type assert root -> *World
	_, err := block.UnmarshalBlock[*World](bcs, NewWorldBlock)
	if err != nil {
		return err
	}
	objTree, err := t.buildObjectTree(bcs)
	if err != nil {
		return err
	}
	graphTree, graphHandle, err := t.buildGraphTree(bcs)
	if err != nil {
		return err
	}
	t.btx, t.bcs = btx, bcs
	if t.graphHd != nil {
		_ = t.graphHd.Close()
	}
	if t.graphTree != nil {
		t.graphTree.Discard()
	}
	if t.objTree != nil {
		t.objTree.Discard()
	}
	t.objTree, t.graphTree, t.graphHd = objTree, graphTree, graphHandle
	return nil
}

// Commit commits the current pending changes to the block cursor.
// updates the WorldState with the new root
func (t *WorldState) Commit() error {
	if !t.write {
		return tx.ErrNotWrite
	}
	select {
	case <-t.ctx.Done():
		return tx.ErrDiscarded
	default:
	}
	w, err := t.GetRoot()
	if err != nil {
		return err
	}
	err = t.flushWorldChanges(w)
	if err != nil || t.btx == nil {
		return err
	}
	_, bcs, err := t.btx.Write(true)
	if err != nil {
		return err
	}
	return t.SetBlockTransaction(t.btx, bcs)
}

// Close closes the store, canceling the context.
func (t *WorldState) Close() error {
	t.ctxCancel()
	return nil
}

// buildObjectTree builds the object tree handle.
func (t *WorldState) buildObjectTree(bcs *block.Cursor) (kvtx.BlockTx, error) {
	return kvtx_block.BuildKvTransaction(t.ctx, bcs.FollowSubBlock(1), true)
}

// buildGraphTree builds the graph tree (kv storage) handle.
func (t *WorldState) buildGraphTree(bcs *block.Cursor) (kvtx.BlockTx, *cayley.Handle, error) {
	ktx, err := kvtx_block.BuildKvTransaction(t.ctx, bcs.FollowSubBlock(2), true)
	if err != nil {
		return nil, nil, err
	}

	// makes frequent NewTx() Get() Discard() calls
	// back it all w/ a single transaction
	graphOpts := make(graph.Options, 1)
	// disable bloom filter: very slow to allocate during tx processing
	graphOpts[cayley_kv.OptNoBloom] = true
	graphHd, err := kvtx_cayley.NewGraph(kvtx.NewTxStore(ktx), graphOpts)
	if err != nil {
		ktx.Discard()
		return nil, nil, err
	}

	return ktx, graphHd, nil
}

// getObjectKeyPrefix returns the key prefix.
func (t *WorldState) getObjectKeyPrefix() []byte {
	return []byte("o/")
}

// buildObjectKey converts a key to a bytes key for the object tree.
func (t *WorldState) buildObjectKey(key string) []byte {
	return bytes.Join([][]byte{
		t.getObjectKeyPrefix(),
		[]byte(key),
	}, nil)
}

// _ is a type assertion
var (
	_ world.WorldState         = ((*WorldState)(nil))
	_ world.ForkableWorldState = ((*WorldState)(nil))
)
