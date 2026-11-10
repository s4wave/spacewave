package refwalk

import "github.com/s4wave/spacewave/db/block"

// Untyped counts the refs whose type the walk cannot name for one reason, such
// as an object type the lookup does not know.
type Untyped struct {
	// Count is the number of such refs.
	Count uint64
	// Sample is the first such ref.
	Sample *block.BlockRef
}
