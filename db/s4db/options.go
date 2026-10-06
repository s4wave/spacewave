package s4db

// Options configures a database.
type Options struct {
	// InlineMax is the largest value stored in the index when creating a
	// file. Zero selects 512 bytes; the most is 1024.
	InlineMax int
	// CacheBytes bounds the memory of decoded index pages. Zero selects
	// 128 MiB.
	CacheBytes int
	// CheckpointMin is the overlay size that always permits a checkpoint;
	// above it a checkpoint starts at half the tree size. Zero selects
	// 16 MiB.
	CheckpointMin int64
	// CheckpointMax is the overlay size that always starts a checkpoint,
	// bounding the log replay at open. A commit waits for the running
	// checkpoint while the overlay exceeds twice this. Zero selects 64 MiB.
	CheckpointMax int64
	// RelocateBudget is the value bytes compaction may move after each
	// commit. Zero selects 4 MiB; negative disables compaction.
	RelocateBudget int64
}

// withDefaults returns o with zero fields set to their defaults.
func (o Options) withDefaults() Options {
	// Size values, the cache, and the overlay.
	if o.InlineMax == 0 {
		o.InlineMax = 512
	}
	o.InlineMax = min(max(o.InlineMax, 0), maxInline)
	if o.CacheBytes == 0 {
		o.CacheBytes = 128 << 20
	}
	if o.CheckpointMin == 0 {
		o.CheckpointMin = 16 << 20
	}
	if o.CheckpointMax == 0 {
		o.CheckpointMax = 64 << 20
	}
	if o.RelocateBudget == 0 {
		o.RelocateBudget = 4 << 20
	}
	return o
}

// checkpointLimit returns the overlay size that starts a checkpoint after
// the checkpoint sb: half the tree, within the configured bounds.
func (o Options) checkpointLimit(sb superblock) int64 {
	return min(max(pageOff(sb.treePages)/2, o.CheckpointMin), o.CheckpointMax)
}
