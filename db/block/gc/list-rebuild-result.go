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
	// UnverifiedRoots names up to sampleSize of the named roots the walk
	// reached them from, so a person sees which holder keeps the rebuild
	// from clearing: a bucket's root, a reader pin or a stage.
	UnverifiedRoots []UnverifiedRoot
	// Suspect counts the suspect records the state counted when the last
	// Finish's join ended: the listed blocks no root reached, each of which a
	// hold refuses or promotes, as Land describes.
	Suspect uint64
	// Passes counts the times Finish walked again because a hold landed
	// between its last pass and the clear; at finishPasses, Finish returned
	// without clearing.
	Passes uint64
	// Cleared reports that every list a root reaches is verified, so the
	// rebuild cleared the Volume's unverified lists mark.
	Cleared bool
	// Unadopted reports that Finish returned the adopted Volume to unadopted
	// with the mark, because the rebuild rewrote a list on it, so the next
	// adoption recounts the lists before anything more frees.
	Unadopted bool
}
