package block_rpc_client

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/net/hash"
)

// ErrBatchExistenceMismatch is returned when a batch response reports a
// different number of existence results than the request held entries.
var ErrBatchExistenceMismatch = errors.New("put block batch: existence results do not match entries")

// BlockStore implements a BlockStore backed by a BlockStore service.
type BlockStore struct {
	// client carries the remote store operations.
	client block_rpc.SRPCBlockStoreClient
	// hashType is the preferred hash type for writes.
	hashType hash.HashType
	// readOnly disables write calls.
	readOnly bool
	// featuresOnce guards the lazy lookup of supportedFeatures.
	featuresOnce sync.Once
	// supportedFeatures caches the remote feature bitmask after the first call.
	supportedFeatures block.StoreFeature
}

// NewBlockStore constructs a new BlockStore.
func NewBlockStore(
	client block_rpc.SRPCBlockStoreClient,
	hashType hash.HashType,
	readOnly bool,
) *BlockStore {
	return &BlockStore{
		client:   client,
		hashType: hashType,
		readOnly: readOnly,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (v *BlockStore) GetHashType() hash.HashType {
	return v.hashType
}

// GetSupportedFeatures returns the native feature bitmask for the remote store.
// The result is cached after the first call: the remote feature set is static
// for the lifetime of the store, and this method is on the hot path.
func (v *BlockStore) GetSupportedFeatures() block.StoreFeature {
	// Fetch and cache the remote feature set once for this store lifetime.
	v.featuresOnce.Do(func() {
		// Retrieve the remote store capabilities before caching them.
		resp, err := v.client.GetSupportedFeatures(context.Background(), &block_rpc.GetSupportedFeaturesRequest{})
		if err != nil {
			v.supportedFeatures = block.StoreFeature_STORE_FEATURE_UNKNOWN
			return
		}

		// Remove write capabilities from the read-only store view.
		features := resp.GetFeatures()
		if v.readOnly {
			features &^= block.StoreFeatureNativeBatchPut
		}
		v.supportedFeatures = features
	})

	return v.supportedFeatures
}

// BeginReadOperation returns the remote store as the scoped read handle.
func (v *BlockStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return v, func() {}, nil
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
func (v *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Reject writes from a read-only client before issuing the RPC.
	if v.readOnly {
		return nil, false, block_store.ErrReadOnly
	}
	resp, err := v.client.PutBlock(ctx, &block_rpc.PutBlockRequest{
		Data:    data,
		PutOpts: opts,
	})
	if err != nil {
		return nil, false, err
	}
	if err := writeFailure(resp.GetError(), resp.GetFailure()); err != nil {
		return nil, false, err
	}
	addedRef := resp.GetRef()
	if err := addedRef.Validate(false); err != nil {
		return nil, false, err
	}
	return addedRef, resp.GetExisted(), nil
}

// PutBlockBatch preserves operation order across bounded remote requests.
// A failed request stops submission, but earlier requests may have committed.
func (v *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Reject read-only clients before building the remote batch request.
	if v.readOnly {
		return nil, block_store.ErrReadOnly
	}

	// Prepare the bounded request buffer and its submission closure.
	// Leave room for the Resource envelope around encoded block messages.
	const batchBytes = 4 << 20
	req := &block_rpc.PutBlockBatchRequest{}
	size := 0
	existed := make([]bool, 0, len(entries))
	flush := func() error {
		// Skip empty requests and stop canceled batch writes.
		if len(req.Entries) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		// Submit the accumulated entries and propagate remote failures.
		resp, err := v.client.PutBlockBatch(ctx, req)
		if err != nil {
			return err
		}
		if err := writeFailure(resp.GetError(), resp.GetFailure()); err != nil {
			return err
		}
		if len(resp.GetExisted()) != len(req.Entries) {
			return ErrBatchExistenceMismatch
		}

		// Keep the committed results and clear the request before the next batch.
		existed = append(existed, resp.GetExisted()...)
		req = &block_rpc.PutBlockBatchRequest{}
		size = 0

		return nil
	}

	// Encode entries in order and submit batches within the packet limit.
	for _, entry := range entries {
		// Stop building the next remote entry when the batch is canceled.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Preserve the block data, outgoing references, and tombstone on the wire.
		wireEntry := &block_rpc.PutBlockBatchEntry{
			Ref:       entry.Ref,
			Data:      entry.Data,
			Refs:      entry.Refs,
			RefsKnown: entry.RefsKnown,
			Tombstone: entry.Tombstone,
		}

		// Flush the preceding batch when the next entry exceeds the packet limit.
		// Ten bytes cover the repeated field's tag and length prefix.
		entrySize := wireEntry.SizeVT() + 10
		if size+entrySize > batchBytes {
			if err := flush(); err != nil {
				return nil, err
			}
		}

		// Accumulate the encoded entry in the current request.
		req.Entries = append(req.Entries, wireEntry)
		size += entrySize
	}

	// Submit the final partial request.
	if err := flush(); err != nil {
		return nil, err
	}
	return existed, nil
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
// The bytes come from another process, so GetBlock checks them against ref and
// returns ErrBlockCorrupt on a mismatch.
func (v *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Fetch the block from the remote store.
	resp, err := v.client.GetBlock(ctx, &block_rpc.GetBlockRequest{
		Ref:        ref.Clone(),
		Background: block.IsBackground(ctx),
	})
	if err != nil {
		return nil, false, err
	}
	if err := readFailure(ref, resp); err != nil {
		return nil, false, err
	}
	if !resp.GetExists() {
		return nil, false, nil
	}

	// Check the remote bytes against ref before returning them.
	if err := block.VerifyRead(ref, resp.GetData(), ""); err != nil {
		return nil, false, err
	}
	return resp.GetData(), true, nil
}

// GetStoredBlock gets a block and its outgoing refs.
func (v *BlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the remote block with its outgoing references.
	resp, err := v.client.GetBlock(ctx, &block_rpc.GetBlockRequest{
		Ref:        ref.Clone(),
		WithRefs:   true,
		Background: block.IsBackground(ctx),
	})
	if err != nil {
		return nil, err
	}

	// Propagate the remote store failure or absence before constructing the block.
	if err := readFailure(ref, resp); err != nil {
		return nil, err
	}
	if !resp.GetExists() {
		return nil, nil
	}
	if err := block.VerifyRead(ref, resp.GetData(), ""); err != nil {
		return nil, err
	}

	return &block.StoredBlock{
		Data:       resp.GetData(),
		Refs:       resp.GetRefs(),
		RefsSource: block.ListedIf(resp.GetRefsKnown()),
	}, nil
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (v *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	resp, err := v.client.GetBlockExists(ctx, &block_rpc.GetBlockExistsRequest{
		Ref: ref.Clone(),
	})
	if err != nil {
		return false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return false, errors.New(errStr)
	}
	return resp.GetExists(), nil
}

// GetBlockExistsBatch requests a remote batch existence check.
func (v *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Query the remote store for every requested block reference.
	resp, err := v.client.GetBlockExistsBatch(ctx, &block_rpc.GetBlockExistsBatchRequest{Refs: refs})
	if err != nil {
		return nil, err
	}

	// Propagate remote failures before validating the existence results.
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}

	// Require one existence result for each requested reference.
	found := resp.GetExists()
	if len(found) != len(refs) {
		return nil, errors.New("block store returned an invalid existence result count")
	}

	return found, nil
}

// StatBlock returns metadata about a block without reading its data.
// Falls back to GetBlockExists and returns Size=-1 (unknown).
// Returns nil, nil if the block does not exist.
func (v *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	found, err := v.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (v *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	// Reject removal from the read-only store view.
	if v.readOnly {
		return block_store.ErrReadOnly
	}

	// Request removal of the remote block and propagate transport failures.
	resp, err := v.client.RmBlock(ctx, &block_rpc.RmBlockRequest{
		Ref: ref.Clone(),
	})
	if err != nil {
		return err
	}

	// Propagate any removal failure reported by the remote store.
	return writeFailure(resp.GetError(), resp.GetFailure())
}

// Sync requests a remote durability barrier and reports whether the remote
// store applied a fence.
func (v *BlockStore) Sync(ctx context.Context) (bool, error) {
	resp, err := v.client.Sync(ctx, &block_rpc.SyncRequest{})
	if err != nil {
		return false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return false, errors.New(errStr)
	}
	return resp.GetFenced(), nil
}

// readFailure returns the error of a block read response with the type the
// serving store returned: a CorruptError for a corrupt read, so the read-through
// and fallback layers above take their corrupt branch for a remote store too.
func readFailure(ref *block.BlockRef, resp *block_rpc.GetBlockResponse) error {
	errStr := resp.GetError()
	if errStr == "" {
		return nil
	}
	if resp.GetFailure() == block_rpc.GetBlockFailure_GET_BLOCK_FAILURE_CORRUPT {
		return &block.CorruptError{Ref: ref}
	}
	return errors.New(errStr)
}

// writeFailure returns the error of a block write response with the identity
// the serving store returned, so a read-only transaction's refusal still
// matches tx.ErrNotWrite on this side of the RPC.
func writeFailure(errStr string, failure block_rpc.WriteBlockFailure) error {
	if errStr == "" {
		return nil
	}
	if failure == block_rpc.WriteBlockFailure_WRITE_BLOCK_FAILURE_NOT_WRITE {
		return &remoteError{text: errStr, cause: tx.ErrNotWrite}
	}
	return errors.New(errStr)
}

// remoteError keeps the serving store's error text and the sentinel it named.
type remoteError struct {
	// text is the serving store's complete error text.
	text string
	// cause is the sentinel the response's failure class names.
	cause error
}

// Error returns the serving store's error text.
func (e *remoteError) Error() string { return e.text }

// Unwrap returns the sentinel the failure class names.
func (e *remoteError) Unwrap() error { return e.cause }

// _ verifies the remote store contract.
var _ block.StoreOps = (*BlockStore)(nil)
