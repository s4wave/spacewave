package packsweep

import "github.com/s4wave/spacewave/db/block"

// CheckBad is one bad block or candidate pack.
type CheckBad struct {
	// Ref is the block, nil for a pack's own failure.
	Ref *block.BlockRef
	// Packs are the candidates holding the block, or the failed pack.
	Packs []string
	// Reason is why it is bad.
	Reason string
}
