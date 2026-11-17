package s4db

import (
	"encoding/binary"
)

// pin records the oldest sequence and manifest version a handle still reads.
type pin struct {
	// seq is the oldest commit a snapshot reads. Value chunks freed by later
	// commits stay allocated.
	seq uint64
	// version is the oldest manifest version a snapshot reads. Run, manifest
	// and log chunks freed by later versions stay allocated.
	version uint64
}

// decodePin reads a slot record, reporting false when it is torn or empty.
func decodePin(b []byte) (pin, bool) {
	if checksum(b[:24]) != binary.LittleEndian.Uint32(b[24:]) {
		return pin{}, false
	}
	p := pin{seq: binary.LittleEndian.Uint64(b[0:]), version: binary.LittleEndian.Uint64(b[8:])}
	return p, true
}

// lower returns the pin covering both p and o.
func (p pin) lower(o pin) pin {
	return pin{seq: min(p.seq, o.seq), version: min(p.version, o.version)}
}

// encode writes p as a slot record.
func (p pin) encode() []byte {
	// Write both marks and their checksum, retaining the slot layout.
	// Bytes 16..24 are unused identity padding, still covered by the checksum.
	b := make([]byte, slotSize)
	binary.LittleEndian.PutUint64(b[0:], p.seq)
	binary.LittleEndian.PutUint64(b[8:], p.version)
	binary.LittleEndian.PutUint32(b[24:], checksum(b[:24]))
	return b
}
