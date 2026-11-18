package space_world

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/hash"
)

// diagReplayAdoptEnv names the environment variable that, set to 1, makes
// Apply replay each operation whose author's World it adopts and log where
// the two Worlds differ, and Install log both Worlds when a checkpoint's
// differs from the replayed one. It is off by default and diagnostic only.
const diagReplayAdoptEnv = "SPACEWAVE_DIAG_REPLAY_ADOPT"

// diagReplayAdopt is set when diagReplayAdoptEnv is 1.
var diagReplayAdopt = os.Getenv(diagReplayAdoptEnv) == "1"

// diagScratchStore keeps every block put through it in memory and reads the
// rest from inner. It defines every StoreOps method itself and embeds nothing,
// so no write, removal or sync reaches inner, and a type assertion for a wider
// store interface fails. Dropping it leaves inner exactly as it was.
type diagScratchStore struct {
	inner  block.StoreOps
	mtx    sync.Mutex
	blocks map[string]*block.StoredBlock
}

func newDiagScratchStore(inner block.StoreOps) *diagScratchStore {
	return &diagScratchStore{inner: inner, blocks: make(map[string]*block.StoredBlock)}
}

func (s *diagScratchStore) lookup(ref *block.BlockRef) *block.StoredBlock {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.blocks[ref.MarshalString()]
}

// GetHashType returns the inner store's hash type.
func (s *diagScratchStore) GetHashType() hash.HashType { return s.inner.GetHashType() }

// GetSupportedFeatures reports no native features.
func (s *diagScratchStore) GetSupportedFeatures() block.StoreFeature { return 0 }

// BeginReadOperation returns the store itself.
func (s *diagScratchStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock keeps the block in memory.
func (s *diagScratchStore) PutBlock(_ context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Address the block.
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}

	// Keep it in memory.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	key := ref.MarshalString()
	_, existed := s.blocks[key]
	s.blocks[key] = &block.StoredBlock{Data: slices.Clone(data), Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	return ref, existed, nil
}

// PutBlockBatch keeps each entry in memory, and drops a tombstoned one from
// memory only.
func (s *diagScratchStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Keep each entry in memory.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	out := make([]bool, len(entries))
	for i, e := range entries {
		key := e.Ref.MarshalString()
		if e.Tombstone {
			delete(s.blocks, key)
			continue
		}
		_, out[i] = s.blocks[key]
		s.blocks[key] = &block.StoredBlock{Data: slices.Clone(e.Data), Refs: e.Refs, RefsKnown: e.RefsKnown}
	}
	return out, nil
}

// GetBlock reads a kept block, then the inner store.
func (s *diagScratchStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if b := s.lookup(ref); b != nil {
		return b.Data, true, nil
	}
	return s.inner.GetBlock(ctx, ref)
}

// GetStoredBlock reads a kept block, then the inner store.
func (s *diagScratchStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if b := s.lookup(ref); b != nil {
		return b, nil
	}
	return s.inner.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks the kept blocks, then the inner store.
func (s *diagScratchStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if s.lookup(ref) != nil {
		return true, nil
	}
	return s.inner.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks each ref as GetBlockExists does.
func (s *diagScratchStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		ok, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = ok
	}
	return out, nil
}

// RmBlock drops a kept block from memory only.
func (s *diagScratchStore) RmBlock(_ context.Context, ref *block.BlockRef) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.blocks, ref.MarshalString())
	return nil
}

// StatBlock stats a kept block, then the inner store.
func (s *diagScratchStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if b := s.lookup(ref); b != nil {
		return &block.BlockStat{Ref: ref, Size: int64(len(b.Data))}, nil
	}
	return s.inner.StatBlock(ctx, ref)
}

// GetRoot reports no root.
func (s *diagScratchStore) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (s *diagScratchStore) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots keeps the entries in memory and names no root anywhere.
func (s *diagScratchStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	_, err := s.PutBlockBatch(ctx, entries)
	return err
}

// Sync does nothing: nothing is ever stored.
func (s *diagScratchStore) Sync(context.Context) (bool, error) { return false, nil }

// _ is a type assertion
var _ block.StoreOps = (*diagScratchStore)(nil)

// diagTxKeys returns the transaction types, operation types and object keys
// tx names, through every batched transaction.
func diagTxKeys(tx *world_block_tx.Tx, types, opTypes, keys *[]string) {
	*types = append(*types, tx.GetTxType().String())
	switch tx.GetTxType() {
	case world_block_tx.TxType_TxType_BATCH:
		for _, sub := range tx.GetTxBatch().GetTxs() {
			diagTxKeys(sub, types, opTypes, keys)
		}
	case world_block_tx.TxType_TxType_APPLY_WORLD_OP:
		*opTypes = append(*opTypes, tx.GetTxApplyWorldOp().GetOperationTypeId())
	case world_block_tx.TxType_TxType_APPLY_OBJECT_OP:
		*opTypes = append(*opTypes, tx.GetTxApplyObjectOp().GetOperationTypeId())
		*keys = append(*keys, tx.GetTxApplyObjectOp().GetObjectKey())
	case world_block_tx.TxType_TxType_CREATE_OBJECT:
		*keys = append(*keys, tx.GetTxCreateObject().GetObjectKey())
	case world_block_tx.TxType_TxType_OBJECT_SET:
		*keys = append(*keys, tx.GetTxObjectSet().GetObjectKey())
	case world_block_tx.TxType_TxType_OBJECT_INC_REV:
		*keys = append(*keys, tx.GetTxObjectIncRev().GetObjectKey())
	case world_block_tx.TxType_TxType_DELETE_OBJECT:
		*keys = append(*keys, tx.GetTxDeleteObject().GetObjectKey())
	}
}

// diagCompareAdopted replays op on prev as another device would, and logs the
// operation when the World differs from adopted, the author's World this
// device took for it. The replay writes only to a diagScratchStore over the
// replay's store and a cursor and engine of its own, never r's, and commits
// nothing anywhere else: the stored World and every block it reaches are what
// they would be without it. It skips payload holds and any refusal handling.
func (a *Adapter) diagCompareAdopted(ctx context.Context, op *sobject.OrderedOperation, prev *InnerState, adopted *InnerState) {
	// Only a transaction carries a World change to compare.
	wop := &WorldOp{}
	if err := wop.UnmarshalVT(op.Data); err != nil {
		return
	}
	txOp := wop.GetApplyTxOp()
	if txOp == nil {
		return
	}

	// Name the operation and what its transaction touches.
	le := a.le.WithField("diag", "replay-adopt").WithField("op-hash", hex.EncodeToString(op.Hash))
	var types, opTypes, keys []string
	diagTxKeys(txOp.GetTx(), &types, &opTypes, &keys)
	le = le.WithField("tx-types", strings.Join(types, ",")).WithField("op-types", strings.Join(opTypes, ","))

	// Run as the author, bounded.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	peerID, err := op.Inner.ParsePeerID()
	if err != nil {
		return
	}
	ctx = withReplay(withOperationAuthor(ctx, op.Author))

	// Replay it on a scratch engine at prev.
	scratch := newDiagScratchStore(a.replay.store())
	replayed, err := diagRun(ctx, a, scratch, prev.GetHeadRef(), func(btx *world_block.EngineTx) error {
		ttx, err := txOp.GetTx().LocateTx()
		if err != nil {
			return err
		}
		_, err = ttx.ExecuteTx(ctx, peerID, waitingLookup(a.lookupOp, btx), btx)
		return err
	})
	if err != nil {
		le.WithError(err).Warn("diag: replay of an adopted operation failed")
		return
	}
	adoptedRoot := adopted.GetHeadRef().GetRootRef()
	if replayed.GetRootRef().EqualsRef(adoptedRoot) {
		le.Debug("diag: adopted World matches its replay")
		return
	}

	// Name the objects that differ among those the transaction names.
	le = le.WithField("adopted-root", adoptedRoot.MarshalString()).
		WithField("replayed-root", replayed.GetRootRef().MarshalString())
	if len(keys) != 0 {
		read := func(head *bucket.ObjectRef) []string {
			var out []string
			_, _ = diagRun(ctx, a, scratch, head, func(btx *world_block.EngineTx) error {
				refs, err := btx.GetObjectRootRefsBatch(ctx, keys)
				for _, r := range refs {
					out = append(out, fmt.Sprintf("%v", r))
				}
				return err
			})
			return out
		}
		replayedHead := prev.GetHeadRef().CloneVT()
		replayedHead.RootRef = replayed.GetRootRef()
		inAdopted, inReplayed := read(adopted.GetHeadRef()), read(replayedHead)
		for i, key := range keys {
			var x, y string
			if i < len(inAdopted) {
				x = inAdopted[i]
			}
			if i < len(inReplayed) {
				y = inReplayed[i]
			}
			if x != y {
				le.WithField("object-key", key).WithField("adopted", x).WithField("replayed", y).Warn("diag: object differs between the adopted World and its replay")
			}
		}
	}
	le.WithField("keys", strings.Join(keys, ",")).Warn("diag: adopted World differs from its replay")
}

// diagRun runs fn on a write transaction of a fresh engine at head over store,
// commits it to store, and returns the head after it. It closes the engine.
func diagRun(ctx context.Context, a *Adapter, store block.StoreOps, head *bucket.ObjectRef, fn func(*world_block.EngineTx) error) (*bucket.ObjectRef, error) {
	// Open an engine of its own at head over store.
	cursor, err := newWorldCursor(ctx, a.le, a.opts.Bus, a.opts.StepFactorySet, store, a.bucketID, nil, head)
	if err != nil {
		return nil, err
	}
	defer cursor.Release()
	engine, err := world_block.NewEngine(ctx, a.le, cursor, a.lookupOp, nil, false, world_block.WithDeferredDurability())
	if err != nil {
		return nil, err
	}
	defer func() { _ = engine.Close() }()

	// Run fn on a write transaction and commit it to store.
	btx, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer btx.Discard()
	if err := fn(btx); err != nil {
		return nil, err
	}
	next, err := btx.CommitBlockTransaction(ctx)
	if err != nil || next == nil {
		return head, err
	}
	return next, nil
}

// diagLogInstallMismatch logs both Worlds when a checkpoint's World differs
// from the replayed one at the covered position.
func (a *Adapter) diagLogInstallMismatch(height uint64, covered int, replayed, checkpoint *InnerState) {
	a.le.WithField("diag", "replay-adopt").
		WithField("checkpoint-height", height).
		WithField("covered", covered).
		WithField("replayed-root", replayed.GetHeadRef().GetRootRef().MarshalString()).
		WithField("checkpoint-root", checkpoint.GetHeadRef().GetRootRef().MarshalString()).
		WithField("transform-equal", replayed.GetHeadRef().GetTransformConf().EqualVT(checkpoint.GetHeadRef().GetTransformConf())).
		WithField("retained-equal", slices.EqualFunc(replayed.GetRetainedRoots(), checkpoint.GetRetainedRoots(), func(x, y *RetainedRoot) bool { return x.EqualVT(y) })).
		WithField("replayed-retained", len(replayed.GetRetainedRoots())).
		WithField("checkpoint-retained", len(checkpoint.GetRetainedRoots())).
		Warn("diag: checkpoint World differs from the replayed World")
}
