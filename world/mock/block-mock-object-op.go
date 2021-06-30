package world_mock

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
)

// MockObjectOpId is the mock object operation identifier.
var MockObjectOpId = "hydra/world/mock/mock-object-op"

// ApplyMockObjectOp applies a mock operation.
func ApplyMockObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	if operationTypeID != MockObjectOpId {
		return false, nil
	}

	mockObjectOp, err := ByteSliceToMockObjectOp(op)
	if err != nil {
		return false, err
	}
	nextMsg := mockObjectOp.GetNextMsg()

	// write the updated object state
	var nref *block.BlockRef
	err = objectHandle.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransaction(nil)
		ex, err := block_mock.UnmarshalExample(bcs)
		if err != nil {
			return err
		}
		if ex == nil {
			ex = &block_mock.Example{}
		}
		ex.Msg = nextMsg
		bcs.SetBlock(ex, true)
		nref, bcs, err = btx.Write(true)
		return err
	})
	if err != nil {
		return false, err
	}

	_, err = objectHandle.SetRootRef(&bucket.ObjectRef{RootRef: nref})
	if err != nil {
		return false, err
	}

	return true, nil
}

// _ is a type assertion
var _ world.ApplyObjectOpFunc = ApplyMockObjectOp

// NewMockObjectOp constructs a new MockObjectOp block.
func NewMockObjectOp(msg string) *MockObjectOp {
	return &MockObjectOp{NextMsg: msg}
}

// NewMockObjectOpBlock constructs a new MockObjectOp block.
func NewMockObjectOpBlock() block.Block {
	return &MockObjectOp{}
}

// ByteSliceToMockObjectOp converts a byte slice block a MockObjectOp.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a MockObjectOp, returns the MockObjectOp.
func ByteSliceToMockObjectOp(blk block.Block) (*MockObjectOp, error) {
	if blk == nil {
		return nil, nil
	}
	var out *MockObjectOp
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		out = &MockObjectOp{}
		if err := out.UnmarshalBlock(nr.GetBytes()); err != nil {
			return nil, err
		}
		return out, nil
	}

	out, ok = blk.(*MockObjectOp)
	if !ok {
		return out, block.ErrUnexpectedType
	}
	return out, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MockObjectOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(m)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MockObjectOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, m)
}

// _ is a type assertion
var _ block.Block = ((*MockObjectOp)(nil))
