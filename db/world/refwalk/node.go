package refwalk

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// Node is a block the walk visits, with what decodes it.
type Node struct {
	// Ref addresses the block.
	Ref *block.BlockRef
	// Ctor constructs the block's type.
	Ctor block.Ctor
	// Xfrm decodes the stored bytes, nil for bytes stored as they are.
	Xfrm block.Transformer
	// World is the World whose objects the block belongs to, read for their
	// types. It is nil above any World.
	World *bucket.ObjectRef
	// Origin is the caller's index of the root the walk entered from, which
	// every block below the root carries.
	Origin int
	// Parent is the ref string of the decoded block that named this one,
	// empty for a root.
	Parent string
	// ParentType is that block's Go type.
	ParentType string
	// values constructs the values of the tree the block belongs to, nil when
	// the block is not a tree node or the tree's values are untyped.
	values block.Ctor
}
