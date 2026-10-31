package s4dbv1

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// superSize is the encoded length of a superblock.
const superSize = 88

// superblock is a checkpoint: the tree root, the key count, the space
// record and the log position after it. Two copies alternate by generation.
type superblock struct {
	// gen selects the newer copy; zero marks a copy never written.
	gen uint64
	// seq is the last commit the tree holds.
	seq uint64
	// root is the tree's root page, zero for an empty tree.
	root uint64
	// count is the key count at seq.
	count uint64
	// treePages is the pages the tree takes.
	treePages uint64
	// space locates the space record.
	space extentRef
	// logPos is the file offset of the first record after seq.
	logPos uint64
	// logEnd is the end of the log run holding logPos.
	logEnd uint64
	// logCrc is the checksum of record seq, which the next continues.
	logCrc uint32
}

// decodeSuperblock reads a superblock, reporting false when it is torn or
// was never written.
func decodeSuperblock(b []byte) (superblock, bool) {
	// Reject a torn record.
	if checksum(b[:superSize-4]) != binary.LittleEndian.Uint32(b[superSize-4:]) {
		return superblock{}, false
	}

	// Decode the fields.
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i*8:]) }
	s := superblock{
		gen:       u(0),
		seq:       u(1),
		root:      u(2),
		count:     u(3),
		treePages: u(4),
		space:     extentRef{off: u(5), n: binary.LittleEndian.Uint32(b[72:]), crc: binary.LittleEndian.Uint32(b[76:])},
		logPos:    u(7),
		logEnd:    u(8),
		logCrc:    binary.LittleEndian.Uint32(b[80:]),
	}
	return s, s.gen != 0
}

// readSuper returns the newer valid superblock.
func readSuper(r io.ReaderAt) (superblock, error) {
	// Decode both copies and keep the higher generation.
	var best superblock
	for i := range uint64(2) {
		b := make([]byte, superSize)
		if _, err := r.ReadAt(b, pageOff(superPage+i)); err != nil {
			return superblock{}, err
		}
		if sb, ok := decodeSuperblock(b); ok && sb.gen > best.gen {
			best = sb
		}
	}
	if best.gen == 0 {
		return superblock{}, errors.Wrap(ErrCorrupt, "no valid superblock")
	}
	return best, nil
}
