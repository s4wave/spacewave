package s4db

import (
	"encoding/binary"
	"os"
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

// op is one key change in a commit record.
type op struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// val is the stored value of a set.
	val value
}

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

// encodeCommit encodes r as a commit record.
func encodeCommit(r *record) []byte {
	// Write the commit's bookkeeping.
	b := make([]byte, recordHeader, 64)
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
			b = append(b, 2)
			continue
		}
		b = appendValue(b, o.val)
	}

	// Write the freed value bytes and seal the record.
	b = binary.AppendUvarint(b, uint64(len(r.frees)))
	for _, f := range r.frees {
		b = binary.AppendUvarint(b, f.off)
		b = binary.AppendUvarint(b, uint64(f.n))
	}
	return seal(b, r.seq, kindCommit)
}

// encodeLink encodes a link record to chunk next.
func encodeLink(seq uint64, next run) []byte {
	b := make([]byte, recordHeader, linkSize)
	b = binary.LittleEndian.AppendUint64(b, next.start)
	b = binary.LittleEndian.AppendUint64(b, next.n)
	return seal(b, seq, kindLink)
}

// seal fills in the record header.
func seal(b []byte, seq uint64, kind byte) []byte {
	// Write the length, sequence, and kind, then the checksum over all
	// after it.
	binary.LittleEndian.PutUint32(b[0:], uint32(len(b)))
	binary.LittleEndian.PutUint64(b[8:], seq)
	b[16] = kind
	binary.LittleEndian.PutUint32(b[4:], checksum(b[8:]))
	return b
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
	r.released.seq, r.released.ckpt = d.uvarint(), d.uvarint()
	r.durable = d.uvarint()
	delta, k := binary.Varint(d.b)
	if k <= 0 {
		return nil, false
	}
	d.b = d.b[k:]
	r.delta = delta

	// Read the key changes and the freed value bytes.
	r.ops = make([]op, d.uvarint())
	for i := range r.ops {
		r.ops[i].key = d.bytes()
		if len(d.b) != 0 && d.b[0] == 2 {
			d.b = d.b[1:]
			r.ops[i].del = true
			continue
		}
		r.ops[i].val = d.value()
	}
	r.frees = make([]extentRef, d.uvarint())
	for i := range r.frees {
		r.frees[i] = extentRef{off: d.uvarint(), n: uint32(d.uvarint())}
	}
	return r, d.err == nil
}

// logReader reads records in order through a window of the file.
type logReader struct {
	// f is the database file.
	f *os.File
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the chunk holding pos.
	end uint64
	// seq is the sequence of the next record.
	seq uint64
	// buf holds file bytes from off.
	buf []byte
	// off is the file offset of buf.
	off uint64
}

// next returns the next record, following links, or false at the end of the
// valid log.
func (l *logReader) next() (*record, bool) {
	for {
		b := l.window()
		r, ok := decodeRecord(b, l.seq)
		if !ok {
			return nil, false
		}
		l.pos += uint64(r.size)
		l.seq++
		if r.kind == kindLink {
			l.pos, l.end = r.next.start*pageSize, (r.next.start+r.next.n)*pageSize
			l.buf = nil
		}
		return r, true
	}
}

// window returns the bytes from pos to the chunk end, reading ahead in
// blocks of at least 64 KiB.
func (l *logReader) window() []byte {
	// Serve a record that the buffer already holds whole.
	if l.pos >= l.off && l.pos+recordHeader <= l.off+uint64(len(l.buf)) {
		b := l.buf[l.pos-l.off:]
		if n := uint64(binary.LittleEndian.Uint32(b)); n <= uint64(len(b)) || l.off+uint64(len(l.buf)) >= l.end {
			return b
		}
	}

	// Refill from pos, reading at least the next record when it is large.
	want := uint64(64 << 10)
	var hdr [4]byte
	if _, err := l.f.ReadAt(hdr[:], int64(l.pos)); err == nil {
		want = max(want, uint64(binary.LittleEndian.Uint32(hdr[:])))
	}
	want = min(want, l.end-l.pos)

	// Read the window.
	l.buf = make([]byte, want)
	n, _ := l.f.ReadAt(l.buf, int64(l.pos))
	l.buf, l.off = l.buf[:n], l.pos
	return l.buf
}
