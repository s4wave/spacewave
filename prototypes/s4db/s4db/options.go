package s4db

// Options configures a database.
type Options struct {
	// InlineMax is the largest value stored in the index when creating a
	// file. Zero selects 512 bytes.
	InlineMax int
	// CachePages bounds the decoded index pages kept in memory. Zero
	// selects 16384 pages.
	CachePages int
	// CheckpointMin is the overlay size that always permits a checkpoint;
	// above it a checkpoint starts at a quarter of the tree. Zero selects
	// 4 MiB.
	CheckpointMin int64
	// CheckpointMax is the overlay size that always starts a checkpoint.
	// Zero selects 64 MiB.
	CheckpointMax int64
	// RelocateBudget is the value bytes compaction may move after each
	// commit. Zero selects 4 MiB; negative disables compaction.
	RelocateBudget int64
}
