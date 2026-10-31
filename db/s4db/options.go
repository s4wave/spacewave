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
