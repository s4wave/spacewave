package block_store_s3

// ReadCounts counts a PackStore's block reads by where their bytes came from.
// Concurrent reads of one block that share a read count once.
type ReadCounts struct {
	// Cached counts the blocks the read cache served, which held their hash.
	Cached uint64
	// CacheRejected counts the read cache's answers whose bytes failed the
	// block's hash; the bucket served each of them instead.
	CacheRejected uint64
	// Bucket counts the blocks read from the bucket with one range read.
	Bucket uint64
	// Packs counts the blocks read through the pack readers, from the bucket
	// or the readers' own memory.
	Packs uint64
	// BatchBlocks counts the blocks ReadBatch read from the bucket.
	BatchBlocks uint64
	// BatchReads counts the range reads ReadBatch made for them, so
	// BatchBlocks/BatchReads is the blocks one read carried.
	BatchReads uint64
	// BatchBytes counts the bytes those reads carried, wanted or not.
	BatchBytes uint64
}
