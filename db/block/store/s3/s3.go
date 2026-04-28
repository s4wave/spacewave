//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// S3Block is a block store on top of an S3 client and base URL prefix.
// Supports any s3-compatible API.
// Stores blocks at {objectPrefix}/{block ref}
type S3Block struct {
	write        bool
	client       *Client
	bucketName   string
	objectPrefix string
	hashType     hash.HashType
}

// NewS3Block builds a new block store on top of a HTTP service.
//
// client cannot be nil
// hashType can be 0 to use the default hash type.
// if write=false, supports read operations only.
func NewS3Block(
	write bool,
	client *Client,
	bucketName,
	objectPrefix string,
	hashType hash.HashType,
) *S3Block {
	return &S3Block{
		write:        write,
		client:       client,
		bucketName:   bucketName,
		objectPrefix: objectPrefix,
		hashType:     hashType,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *S3Block) GetHashType() hash.HashType {
	return b.hashType
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (b *S3Block) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (b *S3Block) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	if !b.write {
		return nil, false, block_store.ErrReadOnly
	}

	// many stores cannot handle empty values
	// add a blanket check here to be sure
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}

	// hash the block
	ref, err = block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58

	// check exists first
	exists, err = b.getKeyExists(ctx, objectKey)
	if err != nil || exists {
		return ref, exists, err
	}

	// create object
	if err := b.client.PutObject(ctx, b.bucketName, objectKey, data, "application/octet-stream"); err != nil {
		return ref, false, err
	}
	return ref, false, nil
}

// PutBlockBatch loops calling PutBlock or RmBlock per entry.
func (b *S3Block) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			if err := b.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		var ref *block.BlockRef
		if entry.Ref != nil {
			ref = entry.Ref.Clone()
		}
		if _, _, err := b.PutBlock(ctx, entry.Data, &block.PutOpts{
			ForceBlockRef: ref,
			Refs:          block.CloneBlockRefs(entry.Refs),
		}); err != nil {
			return err
		}
	}
	return nil
}

// PutBlockBackground forwards to PutBlock.
func (b *S3Block) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.PutBlock(ctx, data, opts)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any unexpected error.
func (b *S3Block) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if ref.GetEmpty() {
		return nil, false, block.ErrEmptyBlockRef
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58

	body, err := b.client.GetObject(ctx, b.bucketName, objectKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, false, err
	}

	// Verify the data matches the block ref.
	dlRef, err := block.BuildBlockRef(
		data,
		&block.PutOpts{HashType: ref.GetHash().GetHashType(), ForceBlockRef: ref},
	)
	if err != nil {
		return nil, false, err
	}
	if !dlRef.EqualsRef(ref) {
		return nil, true, errors.Wrapf(block.ErrBlockRefMismatch, "service returned %s but expected %s", dlRef.MarshalString(), ref.MarshalString())
	}

	return data, true, nil
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any unexpected error.
func (b *S3Block) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if ref.GetEmpty() {
		return false, block.ErrEmptyBlockRef
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58
	return b.getKeyExists(ctx, objectKey)
}

// GetBlockExistsBatch loops calling GetBlockExists per ref.
func (b *S3Block) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := b.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (b *S3Block) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58

	size, err := b.client.HeadObject(ctx, b.bucketName, objectKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (b *S3Block) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if ref.GetEmpty() {
		return block.ErrEmptyBlockRef
	}
	if !b.write {
		return block_store.ErrReadOnly
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58
	err := b.client.DeleteObject(ctx, b.bucketName, objectKey)
	if err != nil && errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Flush returns nil because S3Block has no buffered writes.
func (b *S3Block) Flush(context.Context) error {
	return nil
}

// BeginDeferFlush opens a no-op defer-flush scope.
func (b *S3Block) BeginDeferFlush() {}

// EndDeferFlush closes a no-op defer-flush scope.
func (b *S3Block) EndDeferFlush(context.Context) error {
	return nil
}

// getKeyExists checks if the given object key exists.
func (b *S3Block) getKeyExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := b.client.HeadObject(ctx, b.bucketName, objectKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// _ is a type assertion
var _ block.StoreOps = ((*S3Block)(nil))
