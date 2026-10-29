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
	// A commit waits for the running checkpoint while the overlay exceeds
	// twice this or the log four times. Zero selects 64 MiB.
	CheckpointMax int64
	// RelocateBudget is the bytes a second the background cleaner moves.
	// Negative turns the cleaner off, and zero selects 4 MiB.
	RelocateBudget int64
	// SingleProcess skips watching the file for other processes' commits
	// and holds the file's writer lock from open to close, so a background
	// checkpoint writes its pages without waiting for this handle's
	// writers. Set it only when no other process opens the file: its
	// readers never see another process's writes, and another process's
	// writers wait until this handle closes. It also starts no watcher
	// goroutine outside the database's own lifetime.
	SingleProcess bool
	// ReadOnly opens an existing file without changing its contents: write
	// transactions and every operation that takes the writer lock return
	// ErrReadOnly, the cleaner stays off, and a log torn by a crash is read
	// up to the tear without cutting it. The handle still writes its reader
	// slot, so another process's writer keeps the space it reads. It is the
	// only way to open a file of the previous value format, to copy it.
	ReadOnly bool
	// Families are key prefixes whose values Flows counts apart. A key
	// counts under the first family it starts with.
	Families [][]byte
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

// checkpointLimit returns the overlay memory that starts a checkpoint after
// the checkpoint sb: half the tree, within the configured bounds.
func (o Options) checkpointLimit(sb superblock) int64 {
	return min(max(pageOff(sb.treePages)/2, o.CheckpointMin), o.CheckpointMax)
}
