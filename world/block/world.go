package world_block

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	kvtx_cayley "github.com/aperturerobotics/hydra/kvtx/cayley"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/golang/protobuf/proto"
)

// WorldState implements world state backed by a block graph.
// Note: calls are not concurrency safe. Use Tx if you want a mutex.
type WorldState struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	btx       *block.Transaction // if != nil -> is write tx
	bcs       *block.Cursor

	objTree   kvtx.BlockTx
	graphTree kvtx.BlockTx
	graphHd   *cayley.Handle

	accessWorldState world.AccessWorldStateFunc
	lookupOp         world.LookupOp

	pendingChanges []*block.Cursor // *WorldChange
}

// NewWorldState constructs a new world handle.
// btx can be nil to indicate a read-only tree.
// bcs is located at the root of the world (the World block).
// if bcs is empty, creates a new empty world.
// world and object op handlers manage applying batch operations.
func NewWorldState(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
	accessWorldState world.AccessWorldStateFunc,
	lookupOp world.LookupOp,
) (*WorldState, error) {
	tx := &WorldState{
		btx: btx,
		bcs: bcs,

		accessWorldState: accessWorldState,
		lookupOp:         lookupOp,
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
	bls *bucket_lookup.Cursor,
	accessWorldState world.AccessWorldStateFunc,
	lookupOp world.LookupOp,
) (*WorldState, error) {
	btx, bcs := bls.BuildTransaction(nil)
	return NewWorldState(ctx, btx, bcs, accessWorldState, lookupOp)
}

// GetReadOnly returns if the world handle is read-only.
func (t *WorldState) GetReadOnly() bool {
	return t.btx == nil
}

// GetRootRef returns the current root reference.
func (t *WorldState) GetRootRef() *block.BlockRef {
	return t.bcs.GetRef()
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (t *WorldState) GetSeqno() (uint64, error) {
	w, err := t.getRoot()
	if err != nil {
		return 0, err
	}
	return w.GetLastChange().GetSeqno(), nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *WorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	access := t.accessWorldState
	if access == nil {
		return world.ErrWorldStateUnavailable
	}
	return access(ctx, ref, cb)
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

	subCtx, subCtxCancel := context.WithCancel(t.ctx)
	defer subCtxCancel()

	sysErr, err := op.ApplyWorldOp(subCtx, t, opSender)
	if err != nil {
		return 0, sysErr, err
	}
	seq, err := t.GetSeqno()
	if err != nil {
		return 0, true, err
	}
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
		bcs.GetTransaction(),
		bcs,
		t.accessWorldState,
		t.lookupOp,
	)
}

// SetBlockTransaction loads the state from the given block transaction and cursor.
func (t *WorldState) SetBlockTransaction(btx *block.Transaction, bcs *block.Cursor) error {
	root, err := bcs.Unmarshal(NewWorldBlock)
	if err != nil {
		return err
	}
	if bcs.GetRef().GetEmpty() && root == nil {
		// initialize new world
		root = NewWorldBlock()
		bcs.SetBlock(root, true)
	}
	_, ok := root.(*World)
	if !ok {
		return block.ErrUnexpectedType
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

// Commit commits the current pending changes to the block transaction.
// updates the WorldState with the new root
func (t *WorldState) Commit() error {
	if t.btx == nil {
		return tx.ErrNotWrite
	}
	select {
	case <-t.ctx.Done():
		return tx.ErrDiscarded
	default:
	}
	w, err := t.getRoot()
	if err != nil {
		return err
	}
	err = t.flushWorldChanges(w)
	if err != nil {
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
	graphHd, err := kvtx_cayley.NewGraph(kvtx.NewTxStore(ktx), graph.Options{})
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

// getRoot builds the Root object from the block cursor.
func (t *WorldState) getRoot() (*World, error) {
	wbi, err := t.bcs.Unmarshal(NewWorldBlock)
	if err != nil {
		return nil, err
	}
	w, ok := wbi.(*World)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return w, nil
}

// _ is a type assertion
var (
	_ world.WorldState         = ((*WorldState)(nil))
	_ world.ForkableWorldState = ((*WorldState)(nil))
)
