package s4db

import (
	"encoding/binary"
)

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

// decodePin reads a slot record, reporting false when it is torn or empty.
func decodePin(b []byte) (pin, bool) {
	if checksum(b[:16]) != binary.LittleEndian.Uint32(b[16:]) {
		return pin{}, false
	}
	p := pin{seq: binary.LittleEndian.Uint64(b[0:]), ckpt: binary.LittleEndian.Uint64(b[8:])}
	return p, true
}

// lower returns the pin covering both p and o.
func (p pin) lower(o pin) pin {
	return pin{seq: min(p.seq, o.seq), ckpt: min(p.ckpt, o.ckpt)}
}

// encode writes p as a slot record.
func (p pin) encode() []byte {
	// Write both marks and their checksum.
	b := make([]byte, slotSize)
	binary.LittleEndian.PutUint64(b[0:], p.seq)
	binary.LittleEndian.PutUint64(b[8:], p.ckpt)
	binary.LittleEndian.PutUint32(b[16:], checksum(b[:16]))
	return b
}
