package block

// ReadCounterSnapshot is a point-in-time copy of block read counters.
type ReadCounterSnapshot struct {
	// BlockReadCount is the number of block reads that returned without store error.
	BlockReadCount uint64
	// BlockReadBytes is the number of bytes returned by found block reads.
	BlockReadBytes uint64
	// BlockReadMissCount is the number of block reads that returned not found without store error.
	BlockReadMissCount uint64
}
