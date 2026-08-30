package s4wave_canvas

import "github.com/s4wave/spacewave/db/block"

// NewCanvasNodeBlock constructs a Canvas node block.
func NewCanvasNodeBlock() block.Block {
	return &CanvasNode{}
}

// MarshalBlock marshals the Canvas node to binary.
func (n *CanvasNode) MarshalBlock() ([]byte, error) {
	return n.MarshalVT()
}

// UnmarshalBlock unmarshals the Canvas node from binary.
func (n *CanvasNode) UnmarshalBlock(data []byte) error {
	return n.UnmarshalVT(data)
}

var _ block.Block = (*CanvasNode)(nil)
