package rootwalk

import "github.com/s4wave/spacewave/db/block"

// RootBlock is one root block Walk read.
type RootBlock struct {
	// Ref addresses the block.
	Ref *block.BlockRef
	// Type is the Go type the block decodes as, such as
	// "*space_world.HeldRoots".
	Type string
	// Found is false when RootBlocks lacks the block.
	Found bool
	// DecodeErr is why a found block did not decode as Type.
	DecodeErr error
	// Refs are the refs the decoded block holds, which a write records as
	// its child list.
	Refs []*block.BlockRef
}
