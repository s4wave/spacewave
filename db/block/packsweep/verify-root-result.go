package packsweep

import "github.com/s4wave/spacewave/db/block"

// VerifyRootResult counts the blocks one root added to a verify: those no
// earlier root reached.
type VerifyRootResult struct {
	// Name is the root's name.
	Name string
	// Ref is the root block.
	Ref *block.BlockRef
	// Blocks is the blocks read and checked.
	Blocks uint64
	// Bytes is their bytes.
	Bytes uint64
	// Bad is the blocks missing, unreadable, corrupt or without their refs.
	Bad uint64
}
