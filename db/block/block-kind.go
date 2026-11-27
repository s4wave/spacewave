package block

// BlockKind is an opaque tag a block type declares for its encoded blocks. The
// block package carries it with each write and attaches no meaning to it, so
// a reader of kept blocks, such as a page count, can tell their types apart
// without decoding them. Zero is no kind.
type BlockKind uint8

// BlockWithKind is a block type that declares its kind. A Transaction tags
// each block it writes with it.
type BlockWithKind interface {
	// BlockKind returns the kind of every block of this type.
	BlockKind() BlockKind
}

// KindOf returns the kind blk declares, or zero.
func KindOf(blk any) BlockKind {
	if k, ok := blk.(BlockWithKind); ok {
		return k.BlockKind()
	}
	return 0
}
