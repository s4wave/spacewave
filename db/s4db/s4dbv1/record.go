package s4dbv1

import (
	"encoding/binary"
	"io"
)

// record is one decoded log record.
type record struct {
	// seq is the record sequence.
	seq uint64
	// kind is the record kind.
	kind byte
	// durable is the last commit the writer had flushed when it wrote this
	// record. Recovery checks the values of records past the highest mark.
	durable uint64
	// ops are a commit's key changes.
	ops []op
	// next is the page run a link record continues in.
	next pageRun
	// size is the encoded length.
	size int
	// crc is the record's checksum.
	crc uint32
}

// op is one key change in a commit record.
type op struct {
	// key is the key.
	key []byte
	// val is the value or a tombstone.
	val value
}

// pageRun is a run of consecutive pages.
type pageRun struct {
	// start is the first page.
	start uint64
	// n is the page count.
	n uint64
}

// decodeRecord reads the record at the start of b, reporting false when b
// holds no complete valid record with sequence seq following the record with
// checksum prev.
func decodeRecord(b []byte, seq uint64, prev uint32) (*record, bool) {
	// Check that a whole record with the expected sequence is present.
	if len(b) < recordHeader {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < recordHeader || n > len(b) || binary.LittleEndian.Uint64(b[8:]) != seq {
		return nil, false
	}
	crc := chain(prev, b[8:n])
	if crc != binary.LittleEndian.Uint32(b[4:]) {
		return nil, false
	}

	// A link holds only the next run.
	r := &record{seq: seq, kind: b[16], size: n, crc: crc}
	d := decoder{b: b[recordHeader:n]}
	switch r.kind {
	case kindLink:
		r.next = pageRun{start: d.u64(), n: d.u64()}
	case kindCommit:
		r.decodeCommit(&d)
	default:
		return nil, false
	}
	return r, d.err == nil
}

// decodeCommit reads a commit's body: the released pin, the durable mark,
// the key count delta, the key changes and the freed extents.
func (r *record) decodeCommit(d *decoder) {
	// Skip the released pin; keep the mark; skip the delta.
	d.uvarint()
	d.uvarint()
	r.durable = d.uvarint()
	d.varint()

	// Read the key changes, copying each key and inline value out of the
	// reader's window.
	r.ops = make([]op, d.count())
	for i := range r.ops {
		r.ops[i].key = append([]byte(nil), d.bytes()...)
		v := d.value(d.kind())
		if !v.isRef && !v.del {
			v.inline = append([]byte(nil), v.inline...)
		}
		r.ops[i].val = v
	}

	// Skip the freed extents.
	for range d.count() {
		d.uvarint()
		d.uvarint()
	}
}

// logWindow is the least a log reader reads ahead.
const logWindow = 64 << 10

// logReader reads records in order through a window of the file.
type logReader struct {
	// r reads the file.
	r io.ReaderAt
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the page run holding pos.
	end uint64
	// seq is the sequence of the next record.
	seq uint64
	// crc is the checksum of the previous record.
	crc uint32
	// buf holds file bytes from off.
	buf []byte
	// off is the file offset of buf.
	off uint64
}

// newLogReader returns a reader of the records after superblock sb.
func newLogReader(r io.ReaderAt, sb superblock) *logReader {
	return &logReader{r: r, pos: sb.logPos, end: sb.logEnd, seq: sb.seq + 1, crc: sb.logCrc}
}

// next returns the next record, following links, or false at the end of the
// valid log.
func (l *logReader) next() (*record, bool) {
	// Decode the record at pos.
	r, ok := decodeRecord(l.window(), l.seq, l.crc)
	if !ok {
		return nil, false
	}

	// Advance past it, moving to the next run after a link.
	l.pos += uint64(r.size) // #nosec G115 -- record sizes are not negative.
	l.seq, l.crc = l.seq+1, r.crc
	if r.kind == kindLink {
		l.pos, l.end = r.next.start*pageSize, (r.next.start+r.next.n)*pageSize
		l.buf = nil
	}
	return r, true
}

// window returns the bytes from pos to the run end, reading ahead at least
// logWindow bytes.
func (l *logReader) window() []byte {
	// Serve a record that the buffer already holds whole.
	if l.pos >= l.off && l.pos+recordHeader <= l.off+uint64(len(l.buf)) {
		b := l.buf[l.pos-l.off:]
		n := uint64(binary.LittleEndian.Uint32(b))
		if n <= uint64(len(b)) || l.off+uint64(len(l.buf)) >= l.end {
			return b
		}
	}

	// Read the next header, stopping at the end of the log without filling
	// a window.
	var hdr [recordHeader]byte
	if l.pos+recordHeader > l.end {
		return nil
	}
	if _, err := l.r.ReadAt(hdr[:], int64(l.pos)); err != nil { // #nosec G115 -- a file offset.
		return nil
	}
	if binary.LittleEndian.Uint64(hdr[8:]) != l.seq {
		return nil
	}

	// Refill from pos, reading at least the next record when it is large.
	want := max(logWindow, uint64(binary.LittleEndian.Uint32(hdr[:])))
	want = min(want, l.end-l.pos)
	l.buf = make([]byte, want)
	n, _ := l.r.ReadAt(l.buf, int64(l.pos)) // #nosec G115 -- a file offset.
	l.buf, l.off = l.buf[:n], l.pos
	return l.buf
}
