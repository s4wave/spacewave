package block_store_kvtx

import (
	"context"
	"runtime/trace"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
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

// GetBlock looks up a block in the store.
// Returns data, found, and error.
func (k *KVTxBlock) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-store/kvtx/get-block")
	defer task.End()

	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}

	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-store/kvtx/get-block/new-transaction")
	tx, err := k.store.NewTransaction(taskCtx, false)
	subtask.End()
	if err != nil {
		return nil, false, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/get-block/get")
	data, found, err := tx.Get(taskCtx, key)
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/get-block/discard")
	tx.Discard()
	subtask.End()
	if err != nil || !found {
		return nil, found, err
	}

	// Re-hash the block reference if configured.
	// This significantly reduces performance but improves security.
	// Otherwise, an attacker could place any data at /h/b/{block-ref}.
	if !k.hashGet {
		return data, found, nil
	}

	_, subtask = trace.NewTask(ctx, "hydra/block-store/kvtx/get-block/hash-verify")
	err = ref.VerifyData(data, true)
	subtask.End()
	// Return the data and the error with the hash mismatch.
	// All callers to GetBlock should check the error return value.
	// We return the data here for cases where we want to report the invalid data.
	return data, found, err
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any exceptional error.
func (k *KVTxBlock) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	return tx.Exists(ctx, key)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (k *KVTxBlock) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	key := k.kvkey.GetBlockKey(rm)

	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

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
	rm, err := ref.MarshalKey()
	if err != nil {
		return err
	}
	key := k.kvkey.GetBlockKey(rm)

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

// _ is a type assertion
var _ block.StoreOps = ((*KVTxBlock)(nil))
