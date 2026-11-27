package block_store_s3

import (
	"slices"
	"sync/atomic"
	"time"
)

// ReadCounts counts a PackStore's block reads by where their bytes came from.
// Concurrent reads of one block that share a read count once.
type ReadCounts struct {
	// Cached counts the blocks the read cache served, which held their hash.
	Cached uint64
	// CacheRejected counts the read cache's answers whose bytes failed the
	// block's hash, so the bucket served each of them instead.
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
	// Spanned counts the blocks cut from the spans a subtree walk read to
	// its blocks' reaches, read again without a request.
	Spanned uint64

	// WindowBytes and the other byte counts record the bytes the store
	// downloaded from the bucket, by read mode: Window through the pack
	// readers' range windows of block payloads, Tail for the index tails
	// the pack readers load, Extent by the one range read of a block's
	// extent, Index for the tails and entry objects the indexer and
	// adoption read, Whole for packs read whole, as a compaction does, and
	// Other for the rest, such as the reclaim state.
	WindowBytes, TailBytes, ExtentBytes, IndexBytes, WholeBytes, OtherBytes uint64
	// WindowFetches and TailFetches count the range reads behind WindowBytes
	// and TailBytes.
	WindowFetches, TailFetches uint64
	// WindowLatency, TailLatency and ExtentLatency time the bucket range
	// reads behind WindowBytes, TailBytes and ExtentBytes, failed ones
	// included.
	WindowLatency, TailLatency, ExtentLatency RangeLatency
}

// RangeLatencyBuckets are the upper bounds, in seconds, of a RangeLatency's
// buckets.
var RangeLatencyBuckets = [...]float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// RangeLatency is a histogram of bucket range read times.
type RangeLatency struct {
	// Counts counts the reads in each bucket: Counts[i] those slower than
	// the bound before RangeLatencyBuckets[i] and at most it, and the last
	// those slower than every bound.
	Counts [len(RangeLatencyBuckets) + 1]uint64
	// Seconds is the reads' total time.
	Seconds float64
}

// rangeLatency is the running count of a RangeLatency.
type rangeLatency struct {
	counts [len(RangeLatencyBuckets) + 1]atomic.Uint64
	nanos  atomic.Uint64
}

// observe counts one read that started at start.
func (l *rangeLatency) observe(start time.Time) {
	d := time.Since(start)
	i, _ := slices.BinarySearch(RangeLatencyBuckets[:], d.Seconds())
	l.counts[i].Add(1)
	l.nanos.Add(uint64(max(d, 0))) //nolint:gosec // d is not negative.
}

// snapshot returns the histogram's counts so far.
func (l *rangeLatency) snapshot() RangeLatency {
	var out RangeLatency
	for i := range l.counts {
		out.Counts[i] = l.counts[i].Load()
	}
	out.Seconds = time.Duration(l.nanos.Load()).Seconds() //nolint:gosec // the sum stays far below the overflow.
	return out
}

// readMode records what a bucket download was for, in ReadCounts' byte counts.
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
