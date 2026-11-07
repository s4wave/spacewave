package block_gc

import "github.com/s4wave/spacewave/db/block"

// ListRebuildResult is what a list rebuild verified and wrote.
type ListRebuildResult struct {
	// Verified counts the lists checked against their block's decoded refs.
	Verified uint64
	// Rewritten counts those written because they were missing or differed.
	Rewritten uint64
	// Unverified counts the listed blocks a named root reaches whose list no
	// walk verified, such as a block whose refs could not be decoded. Any
	// keeps the Volume's lists unverified and Adopt refusing.
	Unverified uint64
	// UnverifiedSample names up to sampleSize of them.
	UnverifiedSample []*block.BlockRef
	// Cleared reports that every list a root reaches is verified, so the
	// rebuild cleared the Volume's unverified lists mark.
	Cleared bool
}
