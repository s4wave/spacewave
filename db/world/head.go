package world

import (
	"github.com/s4wave/spacewave/db/block"
)

// Head is a World root an Authority accepted.
type Head struct {
	// Root is the World root block, or nil before the World's first commit.
	Root *block.BlockRef
	// Seqno is the World seqno at Root.
	Seqno uint64
	// Durable reports whether a record the Authority keeps covers Root, so
	// the head survives a restart of every process that served it.
	Durable bool
}
