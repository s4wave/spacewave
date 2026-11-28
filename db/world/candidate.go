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
	// Staged reports that the commit wrote blocks Root reaches to its
	// writer's stage before Admit, outside Entries, as a write too large for
	// its buffer does. The stage holds them only until the writer's
	// transaction ends, so an Authority that keeps the candidate past that
	// holds Root itself before Admit returns.
	Staged bool
	// Operation is the encoded change for an Authority that replays it, or
	// nil for one that does not. The Authority carries it without reading it.
	Operation []byte
	// ConfirmDurable confirms this candidate's change evidence at the decided
	// head. The Authority calls it only after successful durability, before
	// that head is visible through Head, WatchHead or the Admission. It may
	// run under the Authority's lock and must not block or call the Authority.
	// A rebased head may differ from Root. Nil means no evidence to confirm.
	ConfirmDurable func(Head)
}
