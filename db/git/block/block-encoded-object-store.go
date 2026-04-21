package git_block

import (
	"context"

	"github.com/pkg/errors"
	"github.com/restic/chunker"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
)

// NewEncodedObjectStoreBlock builds a new object store block.
func NewEncodedObjectStoreBlock() block.Block {
	return &EncodedObjectStore{}
}

// IsNil returns if the object is nil.
func (r *EncodedObjectStore) IsNil() bool {
	return r == nil
}

// Validate performs cursory validation of the object.
func (r *EncodedObjectStore) Validate() error {
	if err := r.GetKvtxRoot().Validate(); err != nil {
		return errors.Wrap(err, "kvtx_root")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (r *EncodedObjectStore) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *EncodedObjectStore) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// BuildObjectTree builds the iavl tree.
//
// Bcs should be located at EncodedObjectStore.
func (r *EncodedObjectStore) BuildObjectTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, error) {
	return kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), true)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *EncodedObjectStore) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*kvtx_block.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.KvtxRoot = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *EncodedObjectStore) GetSubBlocks() map[uint32]block.SubBlock {
	if r == nil {
		return nil
	}

	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetKvtxRoot()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *EncodedObjectStore) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	switch id {
	case 1:
		return kvtx_block.NewKeyValueStoreSubBlockCtor(&r.KvtxRoot)
	}
	return nil
}

// getOrGenerateChunkerArgs gets or generates the chunking polynomial.
func (r *EncodedObjectStore) getOrGenerateChunkerArgs() (*blob.ChunkerArgs, error) {
	chunkerArgs := r.GetChunkerArgs()
	if chunkerArgs == nil {
		chunkerArgs = &blob.ChunkerArgs{}
	}
	if chunkerArgs.GetChunkerType() == blob.ChunkerType_ChunkerType_RABIN &&
		chunkerArgs.GetRabinArgs().GetPol() != 0 {
		return chunkerArgs, nil
	}

	p, err := chunker.RandomPolynomial()
	if err != nil {
		return nil, err
	}

	pl := uint64(p)
	chunkerArgs.ChunkerType = blob.ChunkerType_ChunkerType_RABIN
	chunkerArgs.RabinArgs = &blob.RabinArgs{Pol: pl}
	r.ChunkerArgs = chunkerArgs
	return chunkerArgs, nil
}

// _ is a type assertion
var (
	_ block.Block              = (*EncodedObjectStore)(nil)
	_ block.BlockWithSubBlocks = (*EncodedObjectStore)(nil)
)
