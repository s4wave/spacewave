package world

import (
	"github.com/s4wave/spacewave/db/block"
)

// Candidate is a commit a writer proposes as the head after Base.
type Candidate struct {
	// Base is the root the commit was built on.
	Base *block.BlockRef
	// Root is the World root after the commit.
	Root *block.BlockRef
	// Seqno is the World seqno at Root.
	Seqno uint64
	// Entries are the blocks the commit wrote, each with its child list.
	// Admit takes them: the Authority stores them, held by a named root, no
	// later than it reports the candidate durable, and keeps them held until
	// the head moves past Root or the Admission is released undecided. The
	// engine serves them from memory until then.
	Entries []*block.PutBatchEntry
	// Operation is the encoded change for an Authority that replays it, or
	// nil for one that does not. The Authority carries it without reading it.
	Operation []byte
}
