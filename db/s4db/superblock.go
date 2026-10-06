package s4db

import (
	"encoding/binary"
)

// superSize is the encoded length of a superblock.
const superSize = 88

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
	// spaceSeq is the last record the saved space includes. A checkpoint
	// built while commits continued saves the space after them, so it can
	// be later than seq.
	spaceSeq uint64
	// logPos is the file offset of the first record after seq.
	logPos uint64
	// logEnd is the end of the log chunk holding logPos.
	logEnd uint64
	// logCrc is the checksum of record seq, which the record at logPos
	// continues.
	logCrc uint32
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

// decodeSuperblock reads a superblock, reporting false when it is torn or
// was never written.
func decodeSuperblock(b []byte) (superblock, bool) {
	// Reject a torn record.
	if checksum(b[:superSize-4]) != binary.LittleEndian.Uint32(b[superSize-4:]) {
		return superblock{}, false
	}

	// Decode the fields; generation zero marks a never written copy.
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	s := superblock{
		gen:       u(0),
		seq:       u(1),
		root:      u(2),
		count:     u(3),
		treePages: u(4),
		space: extentRef{
			off: u(5),
			n:   binary.LittleEndian.Uint32(b[72:]),
			crc: binary.LittleEndian.Uint32(b[76:]),
		},
		spaceSeq: u(6),
		logPos:   u(7),
		logEnd:   u(8),
		logCrc:   binary.LittleEndian.Uint32(b[80:]),
	}
	return s, s.gen != 0
}

// page returns the file page this superblock is written to.
func (s *superblock) page() int64 {
	return int64(superPage + s.gen%2)
}

// encode writes s into a page.
func (s *superblock) encode() []byte {
	// Write the fields, then the checksum over them.
	b := make([]byte, pageSize)
	fields := []uint64{s.gen, s.seq, s.root, s.count, s.treePages, s.space.off, s.spaceSeq, s.logPos, s.logEnd}
	for i, v := range fields {
		binary.LittleEndian.PutUint64(b[i*8:], v)
	}
	binary.LittleEndian.PutUint32(b[72:], s.space.n)
	binary.LittleEndian.PutUint32(b[76:], s.space.crc)
	binary.LittleEndian.PutUint32(b[80:], s.logCrc)
	binary.LittleEndian.PutUint32(b[superSize-4:], checksum(b[:superSize-4]))
	return b
}
