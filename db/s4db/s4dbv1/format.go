// Package s4dbv1 reads a database file the previous s4db engine wrote: a
// commit log over a copy-on-write B+tree with values over the inline bound
// packed into page-allocated extents. It only reads, and it reads once: a
// Reader streams every pair in key order, merging the checkpointed tree
// with the log after it, so the s4db bulk builder can write the file in the
// current format. Every page, record and extent is checked against its
// checksum, and a torn log tail is cut as the previous engine's own recovery
// would have cut it.
package s4dbv1

import (
	"hash/crc32"
)

// pageSize is the unit of the file: the header, the slot page, the
// superblocks and every tree page take one page.
const pageSize = 4096

// Fixed pages at the start of the file.
const (
	// headerPage holds the header.
	headerPage = 0
	// superPage is the first of two alternating superblocks.
	superPage = 2
)

// Lock byte offsets, shared with the previous engine so a Reader excludes
// its processes. The engine held lockWriter for each transaction and a slot
// lock for the life of each handle.
const (
	// lockWriter is the writer lock.
	lockWriter = 1 << 40
	// lockSlot is the first reader slot lock; slot i locks lockSlot+i.
	lockSlot = lockWriter + 1
	// slots is the number of reader slots.
	slots = 128
)

// Header layout: the magic, the layout version, the page size, three feature
// flag words, three algorithm bytes and the inline bound, with a checksum in
// the last four bytes of the page.
const (
	// headerVersion is the layout version the previous engine wrote.
	headerVersion = 1
	// checksumCRC32C identifies CRC-32C.
	checksumCRC32C = 1
	// indexLogTree identifies the log over a copy-on-write B+tree.
	indexLogTree = 1
	// valuesPacked identifies page-allocated value extents.
	valuesPacked = 1
	// maxInline is the largest inline bound the engine allowed.
	maxInline = 1024
)

// magic opens the header.
var magic = [8]byte{'S', '4', 'W', 'A', 'V', 'E', 'D', 'B'}

// Tree page layout: a kind byte, a two-byte entry count, the entries, zero
// padding and a checksum in the last four bytes.
const (
	// pageHeader is the kind byte and the count.
	pageHeader = 3
	// pageInner holds keys and child pages.
	pageInner = 0
	// pageLeaf holds keys and values.
	pageLeaf = 1
)

// Value kinds in leaf entries and commit records.
const (
	// valueInline holds the bytes in the entry.
	valueInline = 0
	// valueRef locates an extent.
	valueRef = 1
	// valueDeleted marks a deleted key in a commit record.
	valueDeleted = 2
)

// Log record layout: a length, a checksum, a sequence and a kind byte, then
// the body. The checksum covers everything after itself and continues from
// the previous record's.
const (
	// recordHeader is the record header length.
	recordHeader = 4 + 4 + 8 + 1
	// kindCommit is a committed write transaction.
	kindCommit = 1
	// kindLink continues the log in another run of pages.
	kindLink = 2
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

// pageOff returns the file offset of page.
func pageOff(page uint64) int64 {
	return int64(page * pageSize) // #nosec G115 -- no file holds 1<<63 bytes.
}
