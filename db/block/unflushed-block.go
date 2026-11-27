package block

// UnflushedBlock is a kept block no open Flush stored, as Unflushed reads it.
type UnflushedBlock struct {
	// Kind is the kind its writer tagged it with.
	Kind BlockKind
	// Size is its data length.
	Size int
	// Refs is its complete child list, shared with the store: read only.
	Refs []*BlockRef
}
