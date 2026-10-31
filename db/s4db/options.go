package s4db

// Options configures a database.
type Options struct {
	// CacheBytes bounds the memory of cached index data. Zero selects the
	// default.
	CacheBytes int
	// CheckpointMax bounds the memory of commits not yet folded into the
	// index. Zero selects the default.
	CheckpointMax int64
	// RelocateBudget disables compaction in the background when negative.
	RelocateBudget int64
	// SingleProcess declares that no other process opens the file.
	SingleProcess bool
	// Families are key prefixes whose values Flows counts apart.
	Families [][]byte
}
