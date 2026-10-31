package s4db

import (
	"encoding/binary"
)

// pin is what a process's open snapshots still read, and which handle holds
// the slot. A slot records the minimum over the process's snapshots.
type pin struct {
	// seq is the oldest commit a snapshot reads. Value chunks freed by later
	// commits stay allocated.
	seq uint64
	// version is the oldest manifest version a snapshot reads. Run, manifest
	// and log chunks freed by later versions stay allocated.
	version uint64
	// id is the writer identity of the handle holding the slot, so a
	// reservation's owner can be told alive or gone when slots are reused.
	id uint64
}

// decodePin reads a slot record, reporting false when it is torn or empty.
func decodePin(b []byte) (pin, bool) {
	if checksum(b[:24]) != binary.LittleEndian.Uint32(b[24:]) {
		return pin{}, false
	}
	p := pin{seq: binary.LittleEndian.Uint64(b[0:]), version: binary.LittleEndian.Uint64(b[8:]), id: binary.LittleEndian.Uint64(b[16:])}
	return p, true
}

// lower returns the pin covering both p and o, keeping p's identity.
func (p pin) lower(o pin) pin {
	return pin{seq: min(p.seq, o.seq), version: min(p.version, o.version), id: p.id}
}

// encode writes p as a slot record.
func (p pin) encode() []byte {
	// Write both marks, the identity and their checksum.
	b := make([]byte, slotSize)
	binary.LittleEndian.PutUint64(b[0:], p.seq)
	binary.LittleEndian.PutUint64(b[8:], p.version)
	binary.LittleEndian.PutUint64(b[16:], p.id)
	binary.LittleEndian.PutUint32(b[24:], checksum(b[:24]))
	return b
}
