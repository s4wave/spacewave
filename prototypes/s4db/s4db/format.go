package s4db

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/pkg/errors"
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
// never cover data, and the kernel drops them when the holding process dies.
const (
	// lockWriter is held for the duration of each write transaction.
	lockWriter = 1 << 40
	// lockSlot is the first reader slot lock; slot i locks lockSlot+i.
	lockSlot = lockWriter + 1
)

// slots is the number of processes that may open the file at once.
const slots = pageSize / slotSize

// slotSize is the length of one reader slot record.
const slotSize = 32

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

// header describes how the file is encoded. It is written once at creation.
type header struct {
	// version is the header layout version.
	version uint32
	// compat, rocompat, and incompat are the feature flags.
	compat, rocompat, incompat uint64
	// checksum, index, and values are the algorithm identifiers.
	checksum, index, values uint8
	// inlineMax is the largest value stored inside the index.
	inlineMax uint32
}

// encode writes h into a page.
func (h *header) encode() []byte {
	// Identify the format and its page size.
	b := make([]byte, pageSize)
	copy(b, magic[:])
	binary.LittleEndian.PutUint32(b[8:], h.version)
	binary.LittleEndian.PutUint32(b[12:], pageSize)

	// Name the features and algorithms the file uses.
	binary.LittleEndian.PutUint64(b[16:], h.compat)
	binary.LittleEndian.PutUint64(b[24:], h.rocompat)
	binary.LittleEndian.PutUint64(b[32:], h.incompat)
	b[40], b[41], b[42] = h.checksum, h.index, h.values
	binary.LittleEndian.PutUint32(b[44:], h.inlineMax)

	// Seal the page.
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	return b
}

// decodeHeader reads and validates a header page.
func decodeHeader(b []byte) (*header, error) {
	// Check the magic and the page checksum.
	if [8]byte(b[:8]) != magic {
		return nil, errors.New("not an s4wave database")
	}
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, errors.New("header checksum mismatch")
	}

	// Decode the fields and refuse what this version cannot read.
	h := &header{
		version:   binary.LittleEndian.Uint32(b[8:]),
		compat:    binary.LittleEndian.Uint64(b[16:]),
		rocompat:  binary.LittleEndian.Uint64(b[24:]),
		incompat:  binary.LittleEndian.Uint64(b[32:]),
		checksum:  b[40],
		index:     b[41],
		values:    b[42],
		inlineMax: binary.LittleEndian.Uint32(b[44:]),
	}
	switch {
	case binary.LittleEndian.Uint32(b[12:]) != pageSize:
		return nil, errors.New("unsupported page size")
	case h.incompat&^incompatKnown != 0:
		return nil, errors.Errorf("unsupported incompat features %#x", h.incompat&^incompatKnown)
	case h.rocompat&^rocompatKnown != 0:
		return nil, errors.Errorf("unsupported ro-compat features %#x", h.rocompat&^rocompatKnown)
	case h.checksum != checksumCRC32C || h.index != indexLogTree || h.values != valuesPacked:
		return nil, errors.New("unsupported algorithm")
	}
	return h, nil
}

// superblock names the last checkpoint. Two copies alternate so a torn write
// leaves the previous one intact.
type superblock struct {
	// gen increases with every checkpoint and selects the newer copy.
	gen uint64
	// seq is the last commit record the checkpoint includes.
	seq uint64
	// root is the root page of the tree, zero when empty.
	root uint64
	// count is the number of keys in the tree.
	count uint64
	// treePages is the number of pages in the tree.
	treePages uint64
	// space locates the encoded allocator state.
	space extentRef
	// logPos is the file offset of the first record after seq.
	logPos uint64
	// logEnd is the end of the log chunk holding logPos.
	logEnd uint64
}

// extentRef locates checksummed bytes in the file.
type extentRef struct {
	// off is the file offset.
	off uint64
	// n is the length.
	n uint32
	// crc is the checksum of the bytes.
	crc uint32
}

// superSize is the encoded length of a superblock.
const superSize = 80

// encode writes s into a page.
func (s *superblock) encode() []byte {
	// Write the fields, then the checksum over them.
	b := make([]byte, pageSize)
	for i, v := range []uint64{s.gen, s.seq, s.root, s.count, s.treePages, s.space.off, s.logPos, s.logEnd} {
		binary.LittleEndian.PutUint64(b[i*8:], v)
	}
	binary.LittleEndian.PutUint32(b[64:], s.space.n)
	binary.LittleEndian.PutUint32(b[68:], s.space.crc)
	binary.LittleEndian.PutUint32(b[superSize-4:], checksum(b[:superSize-4]))
	return b
}

// decodeSuperblock reads a superblock, reporting false when it is torn or
// was never written.
func decodeSuperblock(b []byte) (superblock, bool) {
	// Reject a torn record.
	if checksum(b[:superSize-4]) != binary.LittleEndian.Uint32(b[superSize-4:]) {
		return superblock{}, false
	}

	// Decode the fields; generation zero marks a never written slot.
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	s := superblock{
		gen: u(0), seq: u(1), root: u(2), count: u(3), treePages: u(4),
		space:  extentRef{off: u(5), n: binary.LittleEndian.Uint32(b[64:]), crc: binary.LittleEndian.Uint32(b[68:])},
		logPos: u(6), logEnd: u(7),
	}
	return s, s.gen != 0
}

// pin is what a process's open snapshots still read. A slot records the
// minimum over the process's snapshots.
type pin struct {
	// seq is the oldest commit a snapshot reads. Values freed by later
	// commits stay allocated.
	seq uint64
	// ckpt is the oldest checkpoint whose tree a snapshot reads. Pages freed
	// by later checkpoints stay allocated.
	ckpt uint64
}

// encodeSlot writes p as a slot record.
func encodeSlot(p pin) []byte {
	// Write the pin, then the checksum over it.
	b := make([]byte, slotSize)
	binary.LittleEndian.PutUint64(b[0:], p.seq)
	binary.LittleEndian.PutUint64(b[8:], p.ckpt)
	binary.LittleEndian.PutUint32(b[16:], checksum(b[:16]))
	return b
}

// decodeSlot reads a slot record, reporting false when it is torn.
func decodeSlot(b []byte) (pin, bool) {
	if checksum(b[:16]) != binary.LittleEndian.Uint32(b[16:]) {
		return pin{}, false
	}
	return pin{seq: binary.LittleEndian.Uint64(b[0:]), ckpt: binary.LittleEndian.Uint64(b[8:])}, true
}
