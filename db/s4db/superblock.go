package s4db

import (
	"encoding/binary"
)

// superSize is the encoded length of a superblock.
const superSize = 136

// superblock points at a manifest version and the log after it. Two copies
// alternate by generation so a torn write leaves the previous one intact.
type superblock struct {
	// gen increases with every superblock written and selects the newer copy.
	gen uint64
	// version is the manifest version this superblock describes.
	version uint64
	// manifest locates the encoded manifest.
	manifest ref
	// seq is the last commit the manifest's runs and table include.
	seq uint64
	// logPos is the file offset of the first record after seq.
	logPos uint64
	// logEnd is the end of the log chunk holding logPos.
	logEnd uint64
	// logCrc is the checksum of record seq, which the record at logPos
	// continues.
	logCrc uint32
	// logWriter is the writer identity of record seq, which the record at
	// logPos names as its predecessor.
	logWriter uint64
	// through is the last commit the runs hold. The commits after it up to
	// seq are in the log before logPos and rebuild the memtable at open; the
	// manifest already holds their effect on the table.
	through uint64
	// memPos, memEnd, memCrc and memWriter locate the first record after
	// through, as logPos, logEnd, logCrc and logWriter locate the first after
	// seq.
	memPos, memEnd uint64
	memCrc         uint32
	memWriter      uint64
}

// ref locates checksummed bytes in the file.
type ref struct {
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
		gen:     u(0),
		version: u(1),
		manifest: ref{
			off: u(2),
			n:   binary.LittleEndian.Uint32(b[24:]),
			crc: binary.LittleEndian.Uint32(b[28:]),
		},
		seq:       u(4),
		logPos:    u(5),
		logEnd:    u(6),
		logCrc:    binary.LittleEndian.Uint32(b[56:]),
		logWriter: u(8),
		through:   u(9),
		memPos:    u(10),
		memEnd:    u(11),
		memCrc:    binary.LittleEndian.Uint32(b[96:]),
		memWriter: u(13),
	}
	return s, s.gen != 0
}

// page returns the file page this superblock is written to.
func (s *superblock) page() int64 {
	return int64(superPage + s.gen%2)
}

// encode writes s into a page.
func (s *superblock) encode() []byte {
	// Write the generation, version and manifest location.
	b := make([]byte, pageSize)
	binary.LittleEndian.PutUint64(b[0:], s.gen)
	binary.LittleEndian.PutUint64(b[8:], s.version)
	binary.LittleEndian.PutUint64(b[16:], s.manifest.off)
	binary.LittleEndian.PutUint32(b[24:], s.manifest.n)
	binary.LittleEndian.PutUint32(b[28:], s.manifest.crc)

	// Write the log position, then the checksum over the fields.
	binary.LittleEndian.PutUint64(b[32:], s.seq)
	binary.LittleEndian.PutUint64(b[40:], s.logPos)
	binary.LittleEndian.PutUint64(b[48:], s.logEnd)
	binary.LittleEndian.PutUint32(b[56:], s.logCrc)
	binary.LittleEndian.PutUint64(b[64:], s.logWriter)

	// Write the memtable replay position and seal.
	binary.LittleEndian.PutUint64(b[72:], s.through)
	binary.LittleEndian.PutUint64(b[80:], s.memPos)
	binary.LittleEndian.PutUint64(b[88:], s.memEnd)
	binary.LittleEndian.PutUint32(b[96:], s.memCrc)
	binary.LittleEndian.PutUint64(b[104:], s.memWriter)
	binary.LittleEndian.PutUint32(b[superSize-4:], checksum(b[:superSize-4]))
	return b
}
