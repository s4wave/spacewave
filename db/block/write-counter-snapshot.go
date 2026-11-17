package block

// WriteCounterSnapshot is a point-in-time copy of a WriteCounter.
type WriteCounterSnapshot struct {
	// BlockWriteCount is the number of encoded blocks submitted for storage.
	BlockWriteCount uint64
	// BlockWriteBytes is the number of encoded bytes submitted for storage.
	BlockWriteBytes uint64
}
