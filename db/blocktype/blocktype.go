package blocktype

import "github.com/s4wave/spacewave/db/block"

// BlockType constructs block.Block instances and identifies their type.
type BlockType interface {
	// Constructor builds a new zero-value block.Block instance.
	Constructor() block.Block

	// GetBlockTypeID returns the unique identifier for this block type.
	//
	// Format: "spacewave-chat/channel"
	GetBlockTypeID() string
}

// blockType implements BlockType with a constructor function.
type blockType struct {
	typeID string
	ctor   func() block.Block
}

// New builds the BlockType of *T with the unique identifier typeID, such as
// "spacewave-chat/channel".
func New[T any, PT block.Ptr[T]](typeID string) BlockType {
	return NewFunc(typeID, func() block.Block { return PT(new(T)) })
}

// NewFunc builds the BlockType with the unique identifier typeID that ctor
// constructs.
func NewFunc(typeID string, ctor func() block.Block) BlockType {
	return &blockType{typeID: typeID, ctor: ctor}
}

// Constructor builds a new zero-value block.Block instance.
func (t *blockType) Constructor() block.Block {
	return t.ctor()
}

// GetBlockTypeID returns the unique identifier for this block type.
func (t *blockType) GetBlockTypeID() string {
	return t.typeID
}
