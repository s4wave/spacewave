package volume

import (
	"slices"
	"strings"
	"time"
)

// PutFlow counts the blocks one write path stored that the volume did not
// hold, its puts of blocks the volume held, and the puts it refused, since the
// volume opened.
type PutFlow struct {
	// Path names the write path, such as "fill".
	Path string
	// Blocks is the number of blocks the path stored.
	Blocks uint64
	// Bytes is their total length.
	Bytes uint64
	// Existing is the number of puts of blocks the volume held already.
	Existing uint64
	// Refused is the number of puts refused with block.ErrRefsUnknown for a
	// new block written without its references.
	Refused uint64
}

// ValueFlow counts the values the volume's storage stored outside its index
// under one key family, and the stored values it replaced or deleted there,
// since the volume opened.
type ValueFlow struct {
	// Family names the keys, such as "block" or "graph".
	Family string
	// Written is the length of the values stored.
	Written uint64
	// Freed is the length of the stored values replaced or deleted.
	Freed uint64
}

// VolumeFlows is the data a volume wrote since it opened.
type VolumeFlows struct {
	// VolumeID is the volume ID.
	VolumeID string
	// Puts are the new blocks of each write path.
	Puts []PutFlow
	// Values are the values of each key family, empty when the storage does
	// not count them.
	Values []ValueFlow
	// DeadlistsChecked is the number of due deadlist records the collector
	// applied in shadow after its mark.
	DeadlistsChecked uint64
	// DeadlistLivePages is the number of pages those records would free that
	// the mark reached; any is a deadlist error.
	DeadlistLivePages uint64
	// DeadlistLeakedPages is the number of pages those records kept alive
	// that the mark did not reach, which freeing by the records would leak.
	DeadlistLeakedPages uint64
	// DeadlistCountUnderflows is the number of shared count changes those
	// records made that would take a count below zero; any is an error.
	DeadlistCountUnderflows uint64
	// ReleasesResolved is the number of shared count releases the collector
	// resolved.
	ReleasesResolved uint64
	// ReleaseFreedBlocks is the number of blocks those releases freed.
	ReleaseFreedBlocks uint64
	// ReleaseLiveBlocks is the number of those freed blocks the collector's
	// next mark reached; any is an error in the counts.
	ReleaseLiveBlocks uint64
	// ReleaseUnderflows is the number of releases beyond a block's count and
	// the one that freed it; any means a holder was never counted.
	ReleaseUnderflows uint64
	// RevivedPuts is the number of entries marked as already counted that a
	// put found absent and stored afresh; any is an error in the writer's
	// proof that its kept World holds them.
	RevivedPuts uint64
	// BackgroundWaitMax is the longest a commit waited on the storage's
	// background work, such as a background step holding its writer, zero
	// when the storage does not count it.
	BackgroundWaitMax time.Duration
}

// FlowReporter is implemented by a volume that counts the data it writes.
type FlowReporter interface {
	// GetFlows returns the data the volume wrote since it opened.
	GetFlows() *VolumeFlows
}

// FlowSnapshots returns the flows of every volume RegisterHealth registered
// that counts them, sorted by volume ID.
func FlowSnapshots() []*VolumeFlows {
	// Copy the registered reporters, so no snapshot runs under the lock.
	healthSources.mtx.Lock()
	var reporters []FlowReporter
	for w := range healthSources.set {
		if r, ok := w.(FlowReporter); ok {
			reporters = append(reporters, r)
		}
	}
	healthSources.mtx.Unlock()

	// Snapshot each and sort them.
	out := make([]*VolumeFlows, 0, len(reporters))
	for _, r := range reporters {
		out = append(out, r.GetFlows())
	}
	slices.SortFunc(out, func(a, b *VolumeFlows) int {
		return strings.Compare(a.VolumeID, b.VolumeID)
	})
	return out
}
