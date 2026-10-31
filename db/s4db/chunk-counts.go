package s4db

// ChunkCounts counts a file's chunks by kind.
type ChunkCounts struct {
	// Free counts chunks that hold nothing below the end.
	Free int64
	// Log counts log chunks.
	Log int64
	// Value counts value chunks.
	Value int64
	// Run counts run chunks.
	Run int64
	// Manifest counts manifest chunks.
	Manifest int64
	// Reserved counts chunks a background job reserved.
	Reserved int64
	// Pending counts freed chunks a snapshot or recovery may still read.
	Pending int64
}
