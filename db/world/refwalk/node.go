package refwalk

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// Node is a block the walk visits, with what decodes it.
type Node struct {
	// Ref addresses the block.
	Ref *block.BlockRef
	// Ctor constructs the block's type, nil for an untyped block.
	Ctor block.Ctor
	// Untyped is why the walk has no type for the block, as Walker.Untyped
	// describes, on a walk that scans such a block's bytes for its refs. A
	// block below it carries the same reason. Untyped is empty for a typed
	// block.
	Untyped string
	// Xfrm decodes the stored bytes, nil for bytes stored as they are.
	Xfrm block.Transformer
	// Guess reports that the walker guessed the block's type, as PayloadNode
	// does: the walker keeps the decode only when the block is certain, as
	// certain describes, and otherwise reads no refs and does not descend.
	Guess bool
	// World is the World whose objects the block belongs to, read for their
	// types. It is nil above any World.
	World *bucket.ObjectRef
	// values constructs the values of the tree the block belongs to, nil when
	// the block is not a tree node or the tree's values are untyped.
	values block.Ctor
}
