package s4db

import (
	"hash/crc32"
)

// pageSize is the unit of the header area: the header, the slot page and the
// two superblocks.
const pageSize = 4096

// chunkSize is the unit of allocation: every log, value, run and manifest
// allocation is whole chunks, so free space never fragments.
const chunkSize = 1 << 20

// firstChunk is the first chunk the allocator manages; chunk 0 holds the
// header area.
const firstChunk = 1

// Fixed pages at the start of the file.
const (
	// headerPage holds the header, written once at creation.
	headerPage = 0
	// slotPage holds one reader pin per process slot.
	slotPage = 1
	// superPage is the first of two alternating superblocks.
	superPage = 2
)

// Lock byte offsets. Locks sit far past the end of the file, where they
// never cover data, and the system drops them when the holding process dies.
const (
	// lockWriter is held for the duration of each write transaction and for
	// each background step's record.
	lockWriter = 1 << 40
	// lockSlot is the first reader slot lock; slot i locks lockSlot+i.
	lockSlot = lockWriter + 1
	// lockFlush is held by the process running the flush job.
	lockFlush = lockSlot + slots
	// lockMerge is held by the process running the merge job.
	lockMerge = lockFlush + 1
	// lockClean is held by the process running the cleaner.
	lockClean = lockMerge + 1
	// lockLease is the first of leaseLocks lease locks, one per name hash.
	lockLease = 1 << 41
	// leaseLocks is the number of lease lock bytes.
	leaseLocks = 1 << 40
)

// slotSize is the length of one reader slot record.
const slotSize = 32

// slots is the number of processes that may open the file at once.
const slots = pageSize / slotSize

// inlineMax is the largest value stored in the log record and the run entry;
// a longer value goes to a value chunk.
const inlineMax = 512

// blockSize is a run's data block.
const blockSize = 16 << 10

// restartInterval is the entries between restart points in a data block.
const restartInterval = 16

// filterBitsPerKey sizes a run's Bloom filter.
const filterBitsPerKey = 10

// runTarget is the size a merge aims a run at.
const runTarget = 64 * chunkSize

// levelRatio is how many times larger each level is than the one above.
const levelRatio = 10

// level0Trigger is the number of level 0 runs that starts a merge.
const level0Trigger = 4

// level0Stall is the number of level 0 runs past which background writers
// wait and the merger skips the quiet gate, so a steady commit stream cannot
// grow level 0 without bound.
const level0Stall = 12

// magic opens the header.
var magic = [8]byte{'S', '4', 'W', 'A', 'V', 'E', 'D', 'B'}

// Algorithm identifiers recorded in the header. A reader refuses a file whose
// identifiers it does not implement; master's engine wrote indexLogTree (1)
// and valuesPacked (1), which the Volume converts.
const (
	// checksumCRC32C is CRC-32C (Castagnoli), hardware accelerated on amd64
	// and arm64.
	checksumCRC32C = 1
	// indexLogRuns is a log over a memtable flushed into sorted runs that
	// background merges combine.
	indexLogRuns = 2
	// valuesChunked stores values over inlineMax in whole value chunks with
	// their keys beside them.
	valuesChunked = 2
	// indexLogTree is master's index: a log over a copy-on-write B+tree.
	indexLogTree = 1
	// valuesPacked is master's value storage: page-allocated extents.
	valuesPacked = 1
)

// Feature flags follow the ext4 scheme. An unknown compat flag is ignored, an
// unknown ro-compat flag allows read-only opens, and an unknown incompat flag
// refuses the open.
const (
	// incompatKnown lists the incompat flags this version implements.
	incompatKnown = 0
	// rocompatKnown lists the ro-compat flags this version implements.
	rocompatKnown = 0
)

// crcTable is the CRC-32C table.
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// checksum returns the CRC-32C of b.
func checksum(b []byte) uint32 {
	return crc32.Checksum(b, crcTable)
}

// chain returns the CRC-32C of b continued from prev.
func chain(prev uint32, b []byte) uint32 {
	return crc32.Update(prev, crcTable, b)
}

// chunksFor returns the whole chunks that hold n bytes.
func chunksFor(n uint64) uint64 {
	return (n + chunkSize - 1) / chunkSize
}

// chunkOf returns the chunk holding file offset off.
func chunkOf(off uint64) uint64 {
	return off / chunkSize
}

// chunkStart returns the file offset of chunk c.
func chunkStart(c uint64) uint64 {
	return c * chunkSize
}

// fileOff converts a byte offset or length in the file to the int64 the os
// package takes.
func fileOff(off uint64) int64 {
	return int64(off) // #nosec G115 -- no system holds a file of 1<<63 bytes.
}
