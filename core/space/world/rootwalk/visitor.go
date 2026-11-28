package rootwalk

import (
	"context"

	"github.com/s4wave/spacewave/db/world/refwalk"
)

// Visitor receives the blocks Walk reads.
type Visitor struct {
	// Root receives each root block, absent or undecodable ones included.
	Root func(ctx context.Context, rb RootBlock) error
	// Blocks receives each batch the Walker reads below the root blocks, as
	// refwalk.Walker.Walk passes it.
	Blocks func(ctx context.Context, batch []refwalk.Visit) error
}
