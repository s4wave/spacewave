package block_gc

import "github.com/s4wave/spacewave/db/block"

// ListRebuildResult is what a list rebuild verified and wrote.
type ListRebuildResult struct {
	// Verified counts the lists checked against their block's decoded refs.
	Verified uint64
	// Rewritten counts those written because they were missing or lacked
	// children the decoded refs name.
	Rewritten uint64
	// Dropped counts the blocks whose decoded refs lack a child their stored
	// list names. A stored list can only miss children, so every edge it holds
	// is real, and such a report means a wrong constructor or a decoder bug:
	// the rebuild keeps the stored list and leaves the block unverified.
	Dropped uint64
	// DroppedSample names up to sampleSize of them.
	DroppedSample []*block.BlockRef
	// Unverified counts the listed blocks a named root reaches whose list no
	// walk verified, such as a block whose refs could not be decoded. Any
	// keeps the Volume's lists unverified, and the collector freeing only
	// blocks Land gave birth to since.
	Unverified uint64
	// UnverifiedSample names up to sampleSize of them.
	UnverifiedSample []*block.BlockRef
	// Suspect counts the listed blocks no walk verified and no Land wrote
	// that the last Finish named before clearing the mark, each of which a
	// put checks against its refs before Land trusts its list.
	Suspect uint64
	// Cleared reports that every list a root reaches is verified, so the
	// rebuild cleared the Volume's unverified lists mark.
	Cleared bool
	// Unadopted reports that Finish returned the adopted Volume to unadopted
	// with the mark, because the rebuild rewrote a list on it, so the next
	// adoption recounts the lists before anything more frees.
	Unadopted bool
}
