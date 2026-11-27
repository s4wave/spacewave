package block_gc

import "github.com/s4wave/spacewave/db/block"

// FrontCauseCount counts one cause within the bounded front scan.
type FrontCauseCount struct {
	// Count is the number of front entries with this cause.
	Count int
	// Sample is the first ref with this cause, nil when Count is zero.
	Sample *block.BlockRef
}
