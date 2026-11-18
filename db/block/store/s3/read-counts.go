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

	// The other bytes the store downloaded from the bucket, by read mode:
	// Window through the pack readers' range windows of block payloads, Tail
	// for the index tails the pack readers load, Extent by the one range read
	// of a block's extent, Index for the tails and entry objects the indexer
	// and adoption read, Whole for packs read whole, as a compaction does,
	// and Other for the rest, such as the reclaim state.
	WindowBytes, TailBytes, ExtentBytes, IndexBytes, WholeBytes, OtherBytes uint64
	// WindowFetches and TailFetches count the range reads behind WindowBytes
	// and TailBytes.
	WindowFetches, TailFetches uint64
}

// readMode names what a bucket download was for, in ReadCounts' byte counts.
type readMode int

const (
	readBatch readMode = iota
	readWindow
	readTail
	readExtent
	readIndex
	readWhole
	readOther
	readModes
)
