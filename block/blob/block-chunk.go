package blob

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewChunk constructs a new chunk.
func NewChunk(dataRef *block.BlockRef, size, start uint64) *Chunk {
	return &Chunk{DataRef: dataRef, Size: size, Start: start}
}

// NewChunkBlock builds a new repo ref block.
func NewChunkBlock() block.Block {
	return &Chunk{}
}

// Validate checks the reference.
func (r *Chunk) Validate() error {
	if r.GetDataRef().GetEmpty() || r.GetSize() == 0 {
		return ErrEmptyChunk
	}
	if err := r.GetDataRef().Validate(); err != nil {
		return err
	}

	return nil
}

// FetchData fetches the data reference.
// bcs should be located at chunk
func (r *Chunk) FetchData(bcs *block.Cursor, copyBuf bool) ([]byte, error) {
	var data []byte
	var dataOk bool
	var err error
	currChunkDataCs := r.FollowDataRef(bcs)
	currChunkBlki, _ := currChunkDataCs.GetBlock()
	if currChunkBlki != nil {
		currChunkBlk, ok := currChunkBlki.(*byteslice.ByteSlice)
		if ok {
			data = currChunkBlk.GetBytes()
			dataOk = len(data) != 0
		}
	}
	if !dataOk {
		data, dataOk, err = currChunkDataCs.Fetch()
		if err != nil {
			return nil, err
		}
	}
	if !dataOk {
		return nil, errors.Errorf(
			"chunk data block not found: <%q>",
			currChunkDataCs.GetRef().MarshalString(),
		)
	}
	currChunkSize := r.GetSize()
	if len(data) != int(currChunkSize) {
		return nil, errors.Errorf(
			"expected chunk %s data len %d but got %d",
			currChunkDataCs.GetRef().MarshalString(),
			int(currChunkSize),
			len(data),
		)
	}
	if copyBuf {
		buf := make([]byte, len(data))
		copy(buf, data)
		data = buf
	}
	return data, nil
}

// FollowDataRef follows the data reference.
func (r *Chunk) FollowDataRef(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(1, r.GetDataRef())
}

// MarshalBlock marshals the block to binary.
func (r *Chunk) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Chunk) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *Chunk) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		r.DataRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Chunk) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r == nil {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[1] = r.GetDataRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *Chunk) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return byteslice.NewByteSliceBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*Chunk)(nil))
	_ block.BlockWithRefs = ((*Chunk)(nil))
)
