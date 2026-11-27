package packcopy

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Options configures Copy.
type Options struct {
	// Roots are the blocks to copy, with everything under them.
	Roots []*block.BlockRef
	// Target receives the blocks.
	Target Target
	// Sources serve the blocks the target lacks, tried in order.
	Sources []Source
	// Proofs holds the proofs, across runs. The copy owns every key in it,
	// so pass a prefixed view.
	Proofs kvtx.Store
	// Graph receives a record of each block before its pack is written, nil
	// for none. A pack written with no proof yet is read back on resume and
	// not written again, so its records must already be stored.
	Graph *GraphStream
	// Holes receives each hole once with its reason.
	Holes func(ref *block.BlockRef, reason string) error
	// Reads is the number of reads kept in flight. Zero uses DefaultReads.
	Reads int
	// Uploads is the number of packs written at once, which is what fills a
	// fast link: one pack is one PUT. Each pack holds about three copies
	// of its blocks in memory while it writes. Zero uses the target's batch
	// limits.
	Uploads int
	// Progress observes the counts after each pack, nil for none.
	Progress func(Result)
	// Typer decodes blocks by type, nil for none. With a Typer, a block with
	// unknown refs is decoded for them instead of left a hole.
	Typer Typer
	// ReadTarget reads a block the target holds back from the target when no
	// source serves it. Off, the target is written and never read: a held
	// block comes from a source, which needs no request to the bucket, and a
	// held block no source serves is a missing hole.
	ReadTarget bool
}
