package measure

import "github.com/s4wave/spacewave/db/block"

// Child is a reference with an optional serialized type descriptor.
type Child struct {
	// Ref names the child's stored bytes.
	Ref *block.BlockRef
	// Type is a descriptor understood by Describe, empty when unknown.
	Type []byte
}
