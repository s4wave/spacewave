package blocktype

import "github.com/s4wave/spacewave/db/block"

// BlockType provides construction and identification for a block.Block type.
type BlockType interface {
	// Constructor builds a new zero-value block.Block instance.
	Constructor() block.Block

	// GetBlockTypeID returns the unique identifier for this block type.
	// Format: "github.com/s4wave/spacewave/db/block/mock.Root"
	GetBlockTypeID() string

	// MatchesBlockType checks if a block.Block is of this type.
	MatchesBlockType(b block.Block) bool
}

// blockType implements BlockType
type blockType[T block.Block] struct {
	typeID      string
	constructor func() T
}

// NewBlockType builds a BlockType using generics to reduce boilerplate.
//
// T is the concrete block type, typeID is the unique identifier like
// "github.com/s4wave/spacewave/db/block/mock.Root", and constructor is a
// function that returns a new zero-value instance.
func NewBlockType[T block.Block](typeID string, constructor func() T) BlockType {
	return &blockType[T]{
		typeID:      typeID,
		constructor: constructor,
	}
}

// Constructor builds a new zero-value block.Block instance.
func (t *blockType[T]) Constructor() block.Block {
	return t.constructor()
}

// GetBlockTypeID returns the unique identifier for this block type.
func (t *blockType[T]) GetBlockTypeID() string {
	return t.typeID
}

// MatchesBlockType checks if a block.Block is of this type.
func (t *blockType[T]) MatchesBlockType(b block.Block) bool {
	_, ok := b.(T)
	return ok
}

// _ is a type assertion
var _ BlockType = (*blockType[block.Block])(nil)
