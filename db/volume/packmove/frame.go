package packmove

import (
	"bufio"
	"encoding/binary"
	"hash/crc32"
	"io"

	"github.com/pkg/errors"
)

// Frame kinds of the pack log.
const (
	// framePending opens a batch: its keys, written before the batch uploads.
	framePending byte = 'P'
	// frameCommit closes the pending batch before it: the batch is in the
	// bucket.
	frameCommit byte = 'C'
)

// frameCRC checksums frame payloads.
var frameCRC = crc32.MakeTable(crc32.Castagnoli)

// errTornFrame reports a frame cut short or corrupt, as a killed write leaves
// at the end of a log.
var errTornFrame = errors.New("torn pack log frame")

// logEntry is one block key a pending frame records.
type logEntry struct {
	// key is the block key relative to the block prefix.
	key []byte
	// bad marks a block whose data does not match its hash. It is in no pack.
	bad bool
}

// frame is one decoded record of the pack log.
type frame struct {
	// kind is framePending or frameCommit.
	kind byte
	// through is the last block key the batch examined, relative to the block
	// prefix: a run resumes after it. Blocks it passed without recording are
	// ones Pack leaves to the copy, such as blocks outside the World.
	through []byte
	// entries are the blocks the batch packs and the bad ones it found, in
	// key order, for a pending frame.
	entries []logEntry
	// bytes is the size of the batch's packed block data, for a pending frame.
	bytes uint64
	// size is the frame's length in the file.
	size int64
}

// packed counts the entries that are in the batch's pack.
func (f frame) packed() uint64 {
	var n uint64
	for _, e := range f.entries {
		if !e.bad {
			n++
		}
	}
	return n
}

// appendFrame appends the encoding of a frame to dst: the kind byte, the
// payload length as a uvarint, the payload, then the payload's CRC-32C.
// A pending payload is the examined-through key, the data size, the entry
// count, then each entry as a flag byte, a key length and the key; the
// lengths and counts are uvarints.
func appendFrame(dst []byte, f frame) []byte {
	// Encode the payload of a pending frame; a commit frame has none.
	var payload []byte
	if f.kind == framePending {
		payload = binary.AppendUvarint(payload, uint64(len(f.through)))
		payload = append(payload, f.through...)
		payload = binary.AppendUvarint(payload, f.bytes)
		payload = binary.AppendUvarint(payload, uint64(len(f.entries)))
		for _, e := range f.entries {
			flag := byte(0)
			if e.bad {
				flag = 1
			}
			payload = append(payload, flag)
			payload = binary.AppendUvarint(payload, uint64(len(e.key)))
			payload = append(payload, e.key...)
		}
	}
	dst = append(dst, f.kind)
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	dst = append(dst, payload...)
	return binary.LittleEndian.AppendUint32(dst, crc32.Checksum(payload, frameCRC))
}

// frameReader decodes the frames of a pack log in order.
type frameReader struct {
	// r is the log, buffered.
	r *bufio.Reader
}

// newFrameReader reads frames from r.
func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{r: bufio.NewReaderSize(r, 1<<20)}
}

// next returns the next frame, io.EOF at a clean end, or errTornFrame when
// the log ends inside a frame or a frame fails its checksum.
func (fr *frameReader) next() (frame, error) {
	// Read the kind and payload length.
	kind, err := fr.r.ReadByte()
	if err != nil {
		return frame{}, err
	}
	if kind != framePending && kind != frameCommit {
		return frame{}, errTornFrame
	}
	length, err := binary.ReadUvarint(fr.r)
	if err != nil {
		return frame{}, tornOnEOF(err)
	}

	// Read the payload and its checksum.
	payload := make([]byte, length)
	if _, err := io.ReadFull(fr.r, payload); err != nil {
		return frame{}, tornOnEOF(err)
	}
	var sum [4]byte
	if _, err := io.ReadFull(fr.r, sum[:]); err != nil {
		return frame{}, tornOnEOF(err)
	}
	if binary.LittleEndian.Uint32(sum[:]) != crc32.Checksum(payload, frameCRC) {
		return frame{}, errTornFrame
	}

	// Decode the payload.
	f := frame{kind: kind}
	f.size = int64(1 + uvarintLen(length) + len(payload) + len(sum))
	if kind == frameCommit {
		return f, nil
	}
	if err := f.decodePending(payload); err != nil {
		return frame{}, err
	}
	return f, nil
}

// decodePending decodes the payload of a pending frame into f.
func (f *frame) decodePending(payload []byte) error {
	// Read the examined-through key and the data size.
	var ok bool
	if f.through, payload, ok = cutBytes(payload); !ok {
		return errTornFrame
	}
	var n int
	f.bytes, n = binary.Uvarint(payload)
	if n <= 0 {
		return errTornFrame
	}
	payload = payload[n:]

	// Read the entries.
	count, n := binary.Uvarint(payload)
	if n <= 0 || count > uint64(len(payload)) {
		return errTornFrame
	}
	payload = payload[n:]
	f.entries = make([]logEntry, 0, count)
	for range count {
		if len(payload) == 0 {
			return errTornFrame
		}
		bad := payload[0] == 1
		var key []byte
		if key, payload, ok = cutBytes(payload[1:]); !ok {
			return errTornFrame
		}
		f.entries = append(f.entries, logEntry{key: key, bad: bad})
	}
	return nil
}

// cutBytes splits a length-prefixed byte string off the front of b.
func cutBytes(b []byte) (field, rest []byte, ok bool) {
	length, n := binary.Uvarint(b)
	if n <= 0 || length > uint64(len(b)-n) { //nolint:gosec // n is positive and within b.
		return nil, nil, false
	}
	end := n + int(length) //nolint:gosec // length fits the rest of b.
	return b[n:end], b[end:], true
}

// tornOnEOF reports a log that ends inside a frame as a torn frame.
func tornOnEOF(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errTornFrame
	}
	return err
}

// uvarintLen returns the encoded length of v.
func uvarintLen(v uint64) int {
	var buf [binary.MaxVarintLen64]byte
	return binary.PutUvarint(buf[:], v)
}
