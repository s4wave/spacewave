package block_store_kvtx

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/hash"
)

// KVTxBlock is a block store on top of a kvtx store.
type KVTxBlock struct {
	kvkey    *store_kvkey.KVKey
	store    kvtx.Store
	hashType hash.HashType
	hashGet  bool
}

// NewKVTxBlock constructs a new block store on top of a kvtx store.
//
// hashType can be 0 to use a default value.
// hashGet hashes Get requests for integrity, use if the storage is unreliable or untrusted.
func NewKVTxBlock(
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	hashType hash.HashType,
	hashGet bool,
) *KVTxBlock {
	return &KVTxBlock{
		kvkey:    kvkey,
		store:    store,
		hashType: hashType,
		hashGet:  hashGet,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (k *KVTxBlock) GetHashType() hash.HashType {
	return k.hashType
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (k *KVTxBlock) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeatureNativeBatchPut | block.StoreFeatureNativeBatchExists
}

// BeginReadOperation opens one read-only kvtx transaction for a bounded read scope.
func (k *KVTxBlock) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	scope := &readOperation{
		parent: k,
		tx:     tx,
	}
	return scope, scope.release, nil
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTxBlock) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-store/kvtx/put-block")
	defer task.End()

	if opts == nil {
		opts = &block.PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(k.hashType)

	_, subtask := trace.NewTask(ctx, "hydra/block-store/kvtx/put-block/build-block-ref")
	ref, err = block.BuildBlockRef(data, opts)
	subtask.End()
	if err != nil {
		return nil, false, err
	}
	if forceBlockRef := opts.GetForceBlockRef(); !forceBlockRef.GetEmpty() {
		if !ref.EqualsRef(forceBlockRef) {
			return ref, false, block.ErrBlockRefMismatch
		}
	}

	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-store/kvtx/put-block/new-transaction")
	tx, err := k.store.NewTransaction(taskCtx, true)
	subtask.End()
	if err != nil {
		return ref, false, err
	}
	defer tx.Discard()

	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/put-block/exists")
	exists, err = tx.Exists(taskCtx, key)
	subtask.End()
	if err != nil {
		return ref, false, err
	}
	if exists {
		return ref, true, nil
	}

	// many stores cannot handle empty values
	// add a blanket check here to be sure
	if len(data) == 0 {
		return ref, false, block.ErrEmptyBlock
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/put-block/set")
	err = tx.Set(taskCtx, key, data)
	subtask.End()
	if err != nil {
		return ref, false, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/put-block/commit")
	err = tx.Commit(taskCtx)
	subtask.End()
	return ref, false, err
}

// PutBlockBatch writes all entries in one lower kvtx transaction.
func (k *KVTxBlock) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-store/kvtx/put-block-batch")
	defer task.End()

	ops := make([]putBlockBatchOp, 0, len(entries))
	for _, entry := range entries {
		op, err := k.preparePutBlockBatchOp(entry)
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	if len(ops) == 0 {
		return nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-store/kvtx/put-block-batch/new-transaction")
	tx, err := k.store.NewTransaction(taskCtx, true)
	subtask.End()
	if err != nil {
		return err
	}
	defer tx.Discard()

	for _, op := range ops {
		if op.tombstone {
			if err := tx.Delete(ctx, op.key); err != nil {
				return err
			}
			continue
		}
		exists, err := tx.Exists(ctx, op.key)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := tx.Set(ctx, op.key, op.data); err != nil {
			return err
		}
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/put-block-batch/commit")
	err = tx.Commit(taskCtx)
	subtask.End()
	return err
}

// PutBlockBackground forwards to PutBlock.
func (k *KVTxBlock) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return k.PutBlock(ctx, data, opts)
}

// GetBlock looks up a block in the store.
// Returns data, found, and error.
func (k *KVTxBlock) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	return k.getBlock(ctx, tx, ref)
}

func (k *KVTxBlock) getBlock(ctx context.Context, tx kvtx.TxOps, ref *block.BlockRef) ([]byte, bool, error) {
	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}

	key, err := k.blockKey(ref)
	if err != nil {
		return nil, false, err
	}

	data, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		return nil, found, err
	}

	// Re-hash the block reference if configured.
	// This significantly reduces performance but improves security.
	// Otherwise, an attacker could place any data at /h/b/{block-ref}.
	if !k.hashGet {
		return data, found, nil
	}

	err = ref.VerifyData(data, true)
	// Return the data and the error with the hash mismatch.
	// All callers to GetBlock should check the error return value.
	// We return the data here for cases where we want to report the invalid data.
	return data, found, err
}

type readOperation struct {
	parent *KVTxBlock
	tx     kvtx.Tx
	mtx    sync.Mutex
	closed bool
}

func (r *readOperation) GetHashType() hash.HashType {
	return r.parent.GetHashType()
}

func (r *readOperation) GetSupportedFeatures() block.StoreFeature {
	return r.parent.GetSupportedFeatures()
}

func (r *readOperation) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return r, func() {}, nil
}

func (r *readOperation) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrReadOperationReadOnly
}

func (r *readOperation) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return ErrReadOperationReadOnly
}

func (r *readOperation) PutBlockBackground(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrReadOperationReadOnly
}

func (r *readOperation) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.closed {
		return nil, false, ErrReadOperationClosed
	}
	return r.parent.getBlock(ctx, r.tx, ref)
}

func (r *readOperation) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.closed {
		return false, ErrReadOperationClosed
	}
	return r.parent.getBlockExists(ctx, r.tx, ref)
}

func (r *readOperation) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.closed {
		return nil, ErrReadOperationClosed
	}
	return r.parent.getBlockExistsBatch(ctx, r.tx, refs)
}

func (r *readOperation) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.closed {
		return nil, ErrReadOperationClosed
	}
	return r.parent.statBlock(ctx, r.tx, ref)
}

func (r *readOperation) RmBlock(context.Context, *block.BlockRef) error {
	return ErrReadOperationReadOnly
}

func (r *readOperation) Flush(context.Context) error {
	return nil
}

func (r *readOperation) Sync(context.Context) (bool, error) {
	return true, nil
}

func (r *readOperation) BeginDeferFlush() {}

func (r *readOperation) EndDeferFlush(context.Context) error {
	return nil
}

func (r *readOperation) release() {
	r.mtx.Lock()
	if !r.closed {
		r.closed = true
		r.tx.Discard()
	}
	r.mtx.Unlock()
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any exceptional error.
func (k *KVTxBlock) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	return k.getBlockExists(ctx, tx, ref)
}

func (k *KVTxBlock) getBlockExists(ctx context.Context, tx kvtx.TxOps, ref *block.BlockRef) (bool, error) {
	key, err := k.blockKey(ref)
	if err != nil {
		return false, err
	}

	return tx.Exists(ctx, key)
}

// GetBlockExistsBatch checks all refs in one lower kvtx transaction.
func (k *KVTxBlock) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if len(refs) == 0 {
		return []bool{}, nil
	}
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return k.getBlockExistsBatch(ctx, tx, refs)
}

func (k *KVTxBlock) getBlockExistsBatch(ctx context.Context, tx kvtx.TxOps, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := k.getBlockExists(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (k *KVTxBlock) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return k.statBlock(ctx, tx, ref)
}

func (k *KVTxBlock) statBlock(ctx context.Context, tx kvtx.TxOps, ref *block.BlockRef) (*block.BlockStat, error) {
	key, err := k.blockKey(ref)
	if err != nil {
		return nil, err
	}

	exists, err := tx.Exists(ctx, key)
	if err != nil || !exists {
		return nil, err
	}

	data, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		return nil, err
	}

	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KVTxBlock) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	key, err := k.blockKey(ref)
	if err != nil {
		return err
	}

	tx, err := k.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if err := tx.Delete(ctx, key); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Flush returns nil because KVTxBlock has no buffered writes.
func (k *KVTxBlock) Flush(context.Context) error {
	return nil
}

// Sync reports always-durable: KVTxBlock writes commit synchronously.
func (k *KVTxBlock) Sync(context.Context) (bool, error) {
	return true, nil
}

// BeginDeferFlush opens a no-op defer-flush scope.
func (k *KVTxBlock) BeginDeferFlush() {}

// EndDeferFlush closes a no-op defer-flush scope.
func (k *KVTxBlock) EndDeferFlush(context.Context) error {
	return nil
}

type putBlockBatchOp struct {
	key       []byte
	data      []byte
	tombstone bool
}

func (k *KVTxBlock) preparePutBlockBatchOp(entry *block.PutBatchEntry) (putBlockBatchOp, error) {
	if entry.Tombstone {
		key, err := k.blockKey(entry.Ref)
		if err != nil {
			return putBlockBatchOp{}, err
		}
		return putBlockBatchOp{key: key, tombstone: true}, nil
	}

	var ref *block.BlockRef
	if entry.Ref != nil {
		ref = entry.Ref.Clone()
	}
	opts := &block.PutOpts{
		ForceBlockRef: ref,
		Refs:          block.CloneBlockRefs(entry.Refs),
	}
	opts.HashType = opts.SelectHashType(k.hashType)

	actual, err := block.BuildBlockRef(entry.Data, opts)
	if err != nil {
		return putBlockBatchOp{}, err
	}
	if forceBlockRef := opts.GetForceBlockRef(); !forceBlockRef.GetEmpty() {
		if !actual.EqualsRef(forceBlockRef) {
			return putBlockBatchOp{}, block.ErrBlockRefMismatch
		}
	}
	if len(entry.Data) == 0 {
		return putBlockBatchOp{}, block.ErrEmptyBlock
	}

	key, err := k.blockKey(actual)
	if err != nil {
		return putBlockBatchOp{}, err
	}
	return putBlockBatchOp{key: key, data: entry.Data}, nil
}

func (k *KVTxBlock) blockKey(ref *block.BlockRef) ([]byte, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	return k.kvkey.GetBlockKey(rm), nil
}

// _ is a type assertion
var (
	_ block.StoreOps = ((*KVTxBlock)(nil))
	_ block.StoreOps = ((*readOperation)(nil))
)
