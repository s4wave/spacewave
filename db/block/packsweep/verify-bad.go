package packsweep

import "github.com/s4wave/spacewave/db/block"

// VerifyBad is one block a verify could not read whole.
type VerifyBad struct {
	// Root names the root whose walk reached it first.
	Root string
	// Ref is the block.
	Ref *block.BlockRef
	// Reason is its packcopy hole reason: missing, error, corrupt or
	// refs-unknown.
	Reason string
}
