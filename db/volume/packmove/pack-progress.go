package packmove

// PackProgress counts the work of a pack, including the runs a log resumes.
type PackProgress struct {
	// Packs is the number of committed packs.
	Packs uint64
	// Blocks is the number of blocks in them.
	Blocks uint64
	// Bytes is the size of their data.
	Bytes uint64
	// Bad is the number of blocks left out of every pack because their data
	// does not match their hash, counting those the log recorded earlier.
	Bad uint64
	// Untracked counts, among the blocks this run packed, those the volume's
	// reference graph did not track. They were packed without references.
	Untracked uint64
	// Resumed is the number of blocks the log already held at the start.
	Resumed uint64
	// Outside is the number of blocks the reference graph reaches from roots
	// other than the World's block store, which Pack leaves alone and Switch
	// keeps.
	Outside uint64
	// OutsideBytes is the size of their values.
	OutsideBytes uint64
	// Rest measures the keys outside the block range.
	Rest RestReport
}
