package world_mock

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
)

// MockWorldOpId is the mock world operation identifier.
var MockWorldOpId = "hydra/world/mock/mock-world-op"

// ApplyMockWorldOp applies a mock operation.
func ApplyMockWorldOp(
	ctx context.Context,
	worldHandle world.WorldState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	if operationTypeID != MockWorldOpId {
		return false, nil
	}

	mockWorldOp, err := ByteSliceToMockWorldOp(op)
	if err != nil {
		return false, err
	}

	nextMsg := mockWorldOp.GetNextMsg()
	objectID := mockWorldOp.GetObjectId()

	// re-use the logic for the object op
	objState, err := world.MustGetObject(worldHandle, objectID)
	if err != nil {
		return false, err
	}

	// re-use logic for mock object op
	return ApplyMockObjectOp(
		ctx,
		objState,
		MockObjectOpId,
		NewMockObjectOp(nextMsg),
		opSender,
	)
}

// NewMockWorldOp constructs a new MockWorldOp block.
func NewMockWorldOp(objectID, msg string) *MockWorldOp {
	return &MockWorldOp{ObjectId: objectID, NextMsg: msg}
}

// NewMockWorldOpBlock constructs a new MockWorldOp block.
func NewMockWorldOpBlock() block.Block {
	return &MockWorldOp{}
}

// ByteSliceToMockWorldOp converts a byte slice block a MockWorldOp.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a MockWorldOp, returns the MockWorldOp.
func ByteSliceToMockWorldOp(blk block.Block) (*MockWorldOp, error) {
	if blk == nil {
		return nil, nil
	}
	var out *MockWorldOp
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		out = &MockWorldOp{}
		if err := out.UnmarshalBlock(nr.GetBytes()); err != nil {
			return nil, err
		}
		return out, nil
	}

	out, ok = blk.(*MockWorldOp)
	if !ok {
		return out, block.ErrUnexpectedType
	}
	return out, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MockWorldOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(m)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MockWorldOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, m)
}

// _ is a type assertion
var _ block.Block = ((*MockWorldOp)(nil))
