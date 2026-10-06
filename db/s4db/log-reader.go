package s4db

import (
	"encoding/binary"
	"io"
)

// logWindow is the least a log reader reads ahead.
const logWindow = 64 << 10

// logReader reads records in order through a window of the file.
type logReader struct {
	// r reads the database file.
	r io.ReaderAt
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

// newLogReader returns a reader of the records after seq starting at pos in
// the chunk ending at end.
func newLogReader(r io.ReaderAt, pos, end, seq uint64) *logReader {
	return &logReader{r: r, pos: pos, end: end, seq: seq + 1}
}

// next returns the next record, following links, or false at the end of the
// valid log.
func (l *logReader) next() (*record, bool) {
	// Decode the record at pos.
	r, ok := decodeRecord(l.window(), l.seq)
	if !ok {
		return nil, false
	}

	// Advance past it, moving to the next chunk after a link.
	l.pos += uint64(r.size) // #nosec G115 -- record sizes are not negative.
	l.seq++
	if r.kind == kindLink {
		l.pos, l.end = r.next.start*pageSize, (r.next.start+r.next.n)*pageSize
		l.buf = nil
	}
	return r, true
}

// window returns the bytes from pos to the chunk end, reading ahead at least
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
	// a window: a reader at the end, the usual case, reads only the header.
	var hdr [recordHeader]byte
	if _, err := l.r.ReadAt(hdr[:], fileOff(l.pos)); err != nil {
		return nil
	}
	if binary.LittleEndian.Uint64(hdr[8:]) != l.seq {
		return nil
	}

	// Refill from pos, reading at least the next record when it is large.
	want := max(logWindow, uint64(binary.LittleEndian.Uint32(hdr[:])))
	want = min(want, l.end-l.pos)
	l.buf = make([]byte, want)
	n, _ := l.r.ReadAt(l.buf, fileOff(l.pos))
	l.buf, l.off = l.buf[:n], l.pos
	return l.buf
}
