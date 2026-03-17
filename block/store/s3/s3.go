//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
)

// S3Block is a block store on top of a S3 client and base URL prefix.
// Supports any s3-compatible API.
// Stores blocks at {objectPrefix}/{block ref}
type S3Block struct {
	write        bool
	client       *minio.Client
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
	client *minio.Client,
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
	_, err = b.client.PutObject(ctx, b.bucketName, objectKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	return ref, false, err
}

// GetBlock looks up a block in the store.
// Returns data, found, and any unexpected error.
func (b *S3Block) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if ref.GetEmpty() {
		return nil, false, block.ErrEmptyBlockRef
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58

	obj, err := b.client.GetObject(ctx, b.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, false, err
	}

	data, err := io.ReadAll(obj)
	_ = obj.Close()
	if err != nil {
		if isNotFoundErr(err) {
			return nil, false, nil
		}
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

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (b *S3Block) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}

	refB58 := ref.MarshalString()
	objectKey := b.objectPrefix + refB58

	info, err := b.client.StatObject(ctx, b.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}

	return &block.BlockStat{Ref: ref, Size: info.Size}, nil
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
	err := b.client.RemoveObject(ctx, b.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil && isNotFoundErr(err) {
		return nil
	}
	return err
}

// getKeyExists checks if the given object key exists.
func (b *S3Block) getKeyExists(ctx context.Context, objectKey string) (bool, error) {
	obj, err := b.client.GetObject(ctx, b.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return false, err
	}
	defer obj.Close()

	_, err = obj.Stat()
	if err != nil {
		if isNotFoundErr(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// isNotFoundErr returns if the error is not found.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	minioErr, ok := err.(minio.ErrorResponse)
	if ok && minioErr.Code == "NoSuchKey" {
		return true
	}
	return false
}

// _ is a type assertion
var _ block.StoreOps = ((*S3Block)(nil))
