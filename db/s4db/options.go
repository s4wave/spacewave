package s4db

// Options configures a database.
type Options struct {
	// InlineMax is the largest value stored in the index when creating a
	// file. Zero selects 512 bytes; the most is 1024.
	InlineMax int
	// CacheBytes bounds the memory of decoded index pages. Zero selects
	// 128 MiB.
	CacheBytes int
	// CheckpointMin is the overlay memory that always permits a
	// checkpoint; above it a checkpoint starts at half the tree size. Zero
	// selects 16 MiB.
	CheckpointMin int64
	// CheckpointMax is the overlay memory that always starts a checkpoint,
	// bounding the memory and the log replay at open. A log after the checkpoint of twice
	// the limit also starts one, so rewrites of the same keys stay bounded.
	// Zero selects 64 MiB.
	CheckpointMax int64
	// CheckpointSliceBytes is the overlay memory past which a background
	// checkpoint writes only the oldest commits into the index, such as
	// those of the overlay an open replays from a long log. A slice holds
	// this many bytes or half the overlay, whichever is more. Each slice is
	// a checkpoint of its own: it frees the pages it replaced, shortens the
	// log the next open replays, and starts the next slice while the overlay
	// still calls for one. Slices of commits spread over the whole index
	// rewrite leaves the next slice rewrites again, so taking at least half
	// the overlay bounds that rewriting to about twice a whole checkpoint's
	// and drains the overlay faster than writers grow it. Zero selects
	// CheckpointMax; a negative value builds the whole overlay at once.
	// Compact and Release always checkpoint the whole overlay.
	CheckpointSliceBytes int64
	// CommitWaitMax is the overlay memory past which a background commit
	// waits for the running checkpoint; a log after the checkpoint of twice
	// it also holds them. Zero selects twice CheckpointMax. A larger bound
	// keeps background commits flowing through a slow build, such as the
	// first after open on a cold index, at the cost of memory and log replay
	// at open.
	CommitWaitMax int64
	// RelocateBudget, when negative, disables compaction in the background.
	// Zero selects 4 MiB; other values enable it.
	RelocateBudget int64
	// SingleProcess skips watching the file for other processes' commits
	// and holds the file's writer lock from open to close, so a background
	// checkpoint writes its pages without waiting for this handle's
	// writers. Set it only when no other process opens the file: its
	// readers never see another process's writes, and another process's
	// writers wait until this handle closes. It also starts no watcher
	// goroutine outside the database's own lifetime.
	SingleProcess bool
	// Families are key prefixes whose values Flows counts apart. A key
	// counts under the first family it starts with.
	Families [][]byte
	// ReadOnly opens an existing file to read it beside a process that may
	// be writing it, and never writes the file. The handle takes no writer
	// lock and no reader slot, writes no pin, creates no file, recovers no
	// log, cuts no torn tail, compacts nothing and refuses write
	// transactions. It follows the writer only when Refresh is called.
	//
	// With no pin the writer may reuse pages and values a snapshot reads, so
	// a read can fail its checksum or return an older state than Refresh
	// published. Tx.Stale reports a snapshot the writer may have passed;
	// discard a long read when it does, and retry on ErrCorrupt.
	ReadOnly bool
}

// withDefaults returns o with zero fields set to their defaults.
func (o Options) withDefaults() Options {
	// Size values and the cache.
	if o.InlineMax == 0 {
		o.InlineMax = 512
	}
	o.InlineMax = min(max(o.InlineMax, 0), maxInline)
	if o.CacheBytes == 0 {
		o.CacheBytes = 128 << 20
	}

	// Size the overlay, its checkpoints and the relocation budget.
	if o.CheckpointMin == 0 {
		o.CheckpointMin = 16 << 20
	}
	if o.CheckpointMax == 0 {
		o.CheckpointMax = 64 << 20
	}
	if o.CheckpointSliceBytes == 0 {
		o.CheckpointSliceBytes = o.CheckpointMax
	}
	if o.CommitWaitMax == 0 {
		o.CommitWaitMax = 2 * o.CheckpointMax
	}
	if o.RelocateBudget == 0 {
		o.RelocateBudget = 4 << 20
	}
	return o
}

// checkpointLimit returns the overlay memory that starts a checkpoint after
// the checkpoint sb: half the tree, within the configured bounds.
func (o Options) checkpointLimit(sb superblock) int64 {
	return min(max(pageOff(sb.treePages)/2, o.CheckpointMin), o.CheckpointMax)
}

// checkpointDue reports whether st's overlay, or its log after the
// checkpoint, has grown enough to start a checkpoint. Rewrites of the same
// keys grow the log but not the overlay.
func (o Options) checkpointDue(st *state) bool {
	limit := o.checkpointLimit(st.sb)
	return st.overlayBytes > limit || st.logBytes > 2*limit
}
