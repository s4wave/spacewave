package s4db

import (
	"bytes"
	"encoding/binary"
)

// superSize is the encoded length of a superblock without deltas.
const superSize = 88

// A superblock naming deltas extends the first superSize bytes with the
// delta list and ends its page with a checksum of the page. Its first
// checksum is xored with superDeltaMark, so a reader that knows no deltas
// takes it for a torn copy instead of a tree without them.
const (
	// superDeltaMark marks a superblock with deltas.
	superDeltaMark = 0x44454c54
	// superDeltasAt is the offset of the delta count, the round, and the
	// cursor length, two bytes each.
	superDeltasAt = superSize
	// noCursor is the cursor length of a round no step has merged into.
	noCursor = 0xffff
	// deltaRefSize is the encoded length of a delta ref.
	deltaRefSize = 24
)

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
	// deltas names the deltas above the tree, newest first.
	deltas []deltaRef
	// round is the number of oldest deltas a merge is folding into the
	// tree, or zero.
	round int
	// cursor is the last key the round has merged, nil before its first
	// step.
	cursor []byte
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

// decodeSuperblock reads a superblock from its page, reporting false when it
// is torn or was never written.
func decodeSuperblock(b []byte) (superblock, bool) {
	// Reject a torn record, checking the whole page of one with deltas.
	sum, stored := checksum(b[:superSize-4]), binary.LittleEndian.Uint32(b[superSize-4:])
	withDeltas := sum^superDeltaMark == stored
	if sum != stored && (!withDeltas || checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:])) {
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
	if !withDeltas {
		return s, s.gen != 0
	}

	// Decode the deltas, the round, and its cursor.
	u16 := func(at int) int { return int(binary.LittleEndian.Uint16(b[at:])) }
	n, at := u16(superDeltasAt), superDeltasAt+6
	s.round = u16(superDeltasAt + 2)
	if cl := u16(superDeltasAt + 4); cl != noCursor {
		if cl > MaxKeySize {
			return superblock{}, false
		}
		s.cursor = bytes.Clone(b[at : at+cl])
		at += cl
	}
	if n > maxDeltas || s.round > n {
		return superblock{}, false
	}
	s.deltas = make([]deltaRef, n)
	for i := range s.deltas {
		s.deltas[i] = deltaRef{
			root:  binary.LittleEndian.Uint64(b[at:]),
			head:  binary.LittleEndian.Uint64(b[at+8:]),
			pages: binary.LittleEndian.Uint64(b[at+16:]),
		}
		at += deltaRefSize
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
	sum := checksum(b[:superSize-4])
	if len(s.deltas) == 0 {
		binary.LittleEndian.PutUint32(b[superSize-4:], sum)
		return b
	}

	// Mark the copy as one with deltas, and write their count, the round
	// and its cursor.
	binary.LittleEndian.PutUint32(b[superSize-4:], sum^superDeltaMark)
	binary.LittleEndian.PutUint16(b[superDeltasAt:], uint16(len(s.deltas))) // #nosec G115 -- at most maxDeltas.
	binary.LittleEndian.PutUint16(b[superDeltasAt+2:], uint16(s.round))     // #nosec G115 -- the round is at most maxDeltas.
	at := superDeltasAt + 6
	cl := noCursor
	if s.cursor != nil {
		cl = len(s.cursor)
		at += copy(b[at:], s.cursor)
	}
	binary.LittleEndian.PutUint16(b[superDeltasAt+4:], uint16(cl)) // #nosec G115 -- keys are at most MaxKeySize.

	// Write the refs and check the page.
	for _, d := range s.deltas {
		binary.LittleEndian.PutUint64(b[at:], d.root)
		binary.LittleEndian.PutUint64(b[at+8:], d.head)
		binary.LittleEndian.PutUint64(b[at+16:], d.pages)
		at += deltaRefSize
	}
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	return b
}

// indexPages returns the pages of the tree and its deltas.
func (s *superblock) indexPages() uint64 {
	n := s.treePages
	for _, d := range s.deltas {
		n += d.pages
	}
	return n
}
