//go:build js

// Package block_store_opfs implements a block store backed by OPFS with
// per-file WebLock coordination. Content-addressed and idempotent: puts
// for the same block are safe from any number of concurrent workers.
package block_store_opfs

import (
	"context"
	"runtime/trace"
	"syscall/js"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/opfs/filelock"
	b58 "github.com/mr-tron/base58/base58"
)

// BlockStore stores blocks in OPFS files with per-file WebLock coordination.
// Blocks are base58-encoded filenames organized in 2-char shard directories.
// No ACID transactions: content addressing makes puts idempotent.
type BlockStore struct {
	root       js.Value    // OPFS directory for blocks
	lockPrefix string      // WebLock name prefix (e.g. "vol-id/blocks")
	hashType   hash.HashType
}

// NewBlockStore constructs a block store rooted at the given OPFS directory.
// lockPrefix is used for WebLock names (e.g. "vol-id/blocks").
func NewBlockStore(root js.Value, lockPrefix string, hashType hash.HashType) *BlockStore {
	return &BlockStore{root: root, lockPrefix: lockPrefix, hashType: hashType}
}

// GetHashType returns the preferred hash type for the store.
func (s *BlockStore) GetHashType() hash.HashType {
	return s.hashType
}

// PutBlock puts a block into the store.
// Returns the block ref, whether the block already existed, and any error.
func (s *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-store/opfs/put-block")
	defer task.End()

	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}

	if opts == nil {
		opts = &block.PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.hashType)

	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceRef := opts.GetForceBlockRef(); !forceRef.GetEmpty() {
		if !ref.EqualsRef(forceRef) {
			return ref, false, block.ErrBlockRefMismatch
		}
	}

	key, err := encodeRef(ref)
	if err != nil {
		return nil, false, err
	}
	shard := shardPrefix(key)

	shardDir, err := opfs.GetDirectory(s.root, shard, true)
	if err != nil {
		return nil, false, err
	}

	file, release, err := filelock.AcquireFile(shardDir, key, s.lockPrefix+"/"+shard, true)
	if err != nil {
		return nil, false, err
	}
	defer release()

	// Content-addressed: if the file already has data, the block exists.
	if file.Size() > 0 {
		return ref, true, nil
	}

	if _, err := file.WriteAt(data, 0); err != nil {
		return nil, false, err
	}
	file.Flush()
	return ref, false, nil
}

// GetBlock gets a block by reference.
// Returns data, found, error. Returns nil, false, nil if not found.
func (s *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-store/opfs/get-block")
	defer task.End()

	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}

	key, err := encodeRef(ref)
	if err != nil {
		return nil, false, err
	}
	shard := shardPrefix(key)

	shardDir, err := opfs.GetDirectory(s.root, shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	file, release, err := filelock.AcquireFile(shardDir, key, s.lockPrefix+"/"+shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer release()

	size := file.Size()
	if size == 0 {
		return nil, false, nil
	}

	data := make([]byte, size)
	n, err := file.ReadAt(data, 0)
	if err != nil {
		return nil, false, err
	}
	return data[:n], true, nil
}

// GetBlockExists checks if a block exists without reading its data.
// Uses async file existence check (no per-file lock needed).
func (s *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	key, err := encodeRef(ref)
	if err != nil {
		return false, err
	}
	shard := shardPrefix(key)

	shardDir, err := opfs.GetDirectory(s.root, shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return opfs.FileExists(shardDir, key)
}

// RmBlock deletes a block from the store.
// Does not return an error if the block was not present.
func (s *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	key, err := encodeRef(ref)
	if err != nil {
		return err
	}
	shard := shardPrefix(key)

	shardDir, err := opfs.GetDirectory(s.root, shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil
		}
		return err
	}

	err = opfs.DeleteFile(shardDir, key)
	if opfs.IsNotFound(err) {
		return nil
	}
	return err
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (s *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	key, err := encodeRef(ref)
	if err != nil {
		return nil, err
	}
	shard := shardPrefix(key)

	shardDir, err := opfs.GetDirectory(s.root, shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	f, err := opfs.OpenAsyncFile(shardDir, key)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	size, err := f.Size()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// encodeRef encodes a BlockRef as a base58 string for use as a filename.
func encodeRef(ref *block.BlockRef) (string, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return "", err
	}
	return b58.FastBase58Encoding(rm), nil
}

// shardPrefix returns the 2-char shard directory name for an encoded key.
func shardPrefix(key string) string {
	if len(key) < 2 {
		return "00"
	}
	return key[:2]
}

// _ is a type assertion.
var _ block.StoreOps = (*BlockStore)(nil)
