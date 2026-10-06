package s4db

import (
	"hash/crc32"
)

// pageSize is the unit of allocation, index pages, and hole punching.
const pageSize = 4096

// Fixed pages at the start of the file.
const (
	// headerPage holds the header, written once at creation.
	headerPage = 0
	// slotPage holds one reader pin per process slot.
	slotPage = 1
	// superPage is the first of two alternating superblocks.
	superPage = 2
	// firstPage is the first page the allocator manages.
	firstPage = 4
)

// Lock byte offsets. Locks sit far past the end of the file, where they
// never cover data, and the system drops them when the holding process dies.
const (
	// lockWriter is held for the duration of each write transaction.
	lockWriter = 1 << 40
	// lockSlot is the first reader slot lock; slot i locks lockSlot+i.
	lockSlot = lockWriter + 1
	// lockLease is the first of leaseLocks lease locks, one per name hash.
	lockLease = 1 << 41
	// leaseLocks is the number of lease lock bytes.
	leaseLocks = 1 << 40
)

// slotSize is the length of one reader slot record.
const slotSize = 32

// slots is the number of processes that may open the file at once.
const slots = pageSize / slotSize

// extentPages is the size of a value extent or log chunk when free space
// allows: 1 MiB, large enough that packed writes reach full device speed.
const extentPages = 256

// magic opens the header.
var magic = [8]byte{'S', '4', 'W', 'A', 'V', 'E', 'D', 'B'}

// Algorithm identifiers recorded in the header. A reader refuses a file whose
// identifiers it does not implement.
const (
	// checksumCRC32C is CRC-32C (Castagnoli), hardware accelerated on amd64
	// and arm64.
	checksumCRC32C = 1
	// indexLogTree is a logical commit log over a copy-on-write B+tree
	// written once per checkpoint.
	indexLogTree = 1
	// valuesPacked packs each commit's large values into page-allocated
	// extents with per-page live accounting.
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

// pagesFor returns the pages that hold n bytes.
func pagesFor(n int) uint64 {
	return (uint64(n) + pageSize - 1) / pageSize // #nosec G115 -- byte counts are not negative.
}

// fileOff converts a byte offset or length in the file to the int64 the os
// package takes.
func fileOff(off uint64) int64 {
	return int64(off) // #nosec G115 -- no system holds a file of 1<<63 bytes.
}

// pageOff returns the byte offset of page, or the bytes in that many pages.
func pageOff(page uint64) int64 {
	return fileOff(page * pageSize)
}
