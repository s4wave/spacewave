package s4wave_canvas

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// NewCanvasStateBlock constructs a new CanvasState block.
func NewCanvasStateBlock() block.Block {
	return &CanvasState{}
}

// UnmarshalCanvasState unmarshals a canvas state from a cursor.
// If empty, returns nil, nil.
func UnmarshalCanvasState(ctx context.Context, bcs *block.Cursor) (*CanvasState, error) {
	return block.UnmarshalBlock[*CanvasState](ctx, bcs, NewCanvasStateBlock)
}

// Validate performs cursory checks on the CanvasState block.
func (s *CanvasState) Validate() error {
	return nil
}

// MarshalBlock marshals the block to binary.
func (s *CanvasState) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (s *CanvasState) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Clone clones the canvas state.
func (s *CanvasState) Clone() *CanvasState {
	if s == nil {
		return nil
	}
	return s.CloneVT()
}

// _ is a type assertion
var _ block.Block = (*CanvasState)(nil)
