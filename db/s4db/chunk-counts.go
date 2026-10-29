package s4db

// ChunkCounts counts a file's chunks by kind.
type ChunkCounts struct {
	// Free counts the chunks that hold nothing.
	Free int64
	// Log counts the log chunks.
	Log int64
	// Index counts the chunks of index pages and space records.
	Index int64
	// Value counts the value chunks, including each chunk a long value
	// spans.
	Value int64
}
