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
	// checked against its mark.
	DeadlistsChecked uint64
	// DeadlistLivePages is the number of pages those records listed as dead
	// that the mark reached.
	DeadlistLivePages uint64
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
