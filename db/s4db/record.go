package s4db

import (
	"encoding/binary"
)

// Record kinds.
const (
	// kindCommit is a committed write transaction.
	kindCommit = 1
	// kindLink continues the log in another chunk.
	kindLink = 2
)

// recordHeader is the length, checksum, sequence, and kind of a record. The
// checksum covers everything after itself.
const recordHeader = 4 + 4 + 8 + 1

// linkSize is the length of a link record.
const linkSize = recordHeader + 16

// record is one decoded log record.
type record struct {
	// seq is the record sequence.
	seq uint64
	// kind is the record kind.
	kind byte
	// ops are the commit's key changes in key order.
	ops []op
	// frees are the value bytes the commit freed.
	frees []extentRef
	// released marks the freed runs the writer returned to free space:
	// value runs through seq and page runs through ckpt.
	released pin
	// durable is the last commit the writer had flushed when it wrote this
	// record.
	durable uint64
	// delta is the change in key count.
	delta int64
	// next is the chunk a link record continues in.
	next run
	// size is the encoded length.
	size int
}

// op is one key change in a commit record.
type op struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// val is the stored value of a set.
	val value
}

// newLink returns a link record to chunk next.
func newLink(seq uint64, next run) *record {
	return &record{seq: seq, kind: kindLink, next: next}
}

// decodeRecord reads the record at the start of b, reporting false when b
// holds no complete valid record with sequence seq.
func decodeRecord(b []byte, seq uint64) (*record, bool) {
	// Check that a whole record with the expected sequence is present.
	if len(b) < recordHeader {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < recordHeader || n > len(b) || binary.LittleEndian.Uint64(b[8:]) != seq {
		return nil, false
	}
	if checksum(b[8:n]) != binary.LittleEndian.Uint32(b[4:]) {
		return nil, false
	}

	// A link holds only the next chunk.
	r := &record{seq: seq, kind: b[16], size: n}
	d := decoder{b: b[recordHeader:n]}
	if r.kind == kindLink {
		r.next = run{start: d.u64(), n: d.u64()}
		return r, d.err == nil
	}

	// Read the commit's bookkeeping.
	r.released = pin{seq: d.uvarint(), ckpt: d.uvarint()}
	r.durable = d.uvarint()
	r.delta = d.varint()

	// Read the key changes.
	r.ops = make([]op, d.count())
	for i := range r.ops {
		r.ops[i].key = d.bytes()
		kind := d.kind()
		if kind == valueDeleted {
			r.ops[i].del = true
			continue
		}
		r.ops[i].val = d.value(kind)
	}

	// Read the freed value bytes.
	r.frees = make([]extentRef, d.count())
	for i := range r.frees {
		r.frees[i] = extentRef{off: d.uvarint(), n: uint32(d.uvarint())} // #nosec G115 -- encode wrote a uint32.
	}
	return r, d.err == nil
}

// encode returns the encoding of r.
func (r *record) encode() []byte {
	// A link holds only the next chunk.
	if r.kind == kindLink {
		b := make([]byte, recordHeader, linkSize)
		b = binary.LittleEndian.AppendUint64(b, r.next.start)
		b = binary.LittleEndian.AppendUint64(b, r.next.n)
		return r.seal(b)
	}

	// Write the commit's bookkeeping.
	b := make([]byte, recordHeader, r.bound())
	b = binary.AppendUvarint(b, r.released.seq)
	b = binary.AppendUvarint(b, r.released.ckpt)
	b = binary.AppendUvarint(b, r.durable)
	b = binary.AppendVarint(b, r.delta)

	// Write the key changes.
	b = binary.AppendUvarint(b, uint64(len(r.ops)))
	for _, o := range r.ops {
		b = binary.AppendUvarint(b, uint64(len(o.key)))
		b = append(b, o.key...)
		if o.del {
			b = append(b, valueDeleted)
			continue
		}
		b = o.val.append(b)
	}

	// Write the freed value bytes and seal the record.
	b = binary.AppendUvarint(b, uint64(len(r.frees)))
	for _, f := range r.frees {
		b = binary.AppendUvarint(b, f.off)
		b = binary.AppendUvarint(b, uint64(f.n))
	}
	return r.seal(b)
}

// bound returns an upper bound on the encoded length of a commit record,
// counting its frees before the commit adds them.
func (r *record) bound() int {
	n := recordHeader + 4*binary.MaxVarintLen64 + 2*binary.MaxVarintLen64
	for _, o := range r.ops {
		n += binary.MaxVarintLen64 + len(o.key) + o.val.size()
	}
	return n + 2*binary.MaxVarintLen64*len(r.frees)
}

// seal fills in the record header of b.
func (r *record) seal(b []byte) []byte {
	// Write the length, sequence, and kind, then the checksum over all
	// after it.
	binary.LittleEndian.PutUint32(b[0:], uint32(len(b))) // #nosec G115 -- commit bounds records by maxRecord.
	binary.LittleEndian.PutUint64(b[8:], r.seq)
	b[16] = r.kind
	binary.LittleEndian.PutUint32(b[4:], checksum(b[8:]))
	return b
}
