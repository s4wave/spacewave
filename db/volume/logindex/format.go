package logindex

import (
	"encoding/binary"
	"hash/crc32"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/memtable"
)

// Device file names.
const (
	// manifestName is the file holding the two manifest slots.
	manifestName = "manifest"
	// logPrefix starts the name of every log file.
	logPrefix = "log-"
	// checkpointPrefix starts the name of every checkpoint file.
	checkpointPrefix = "ckpt-"
)

// slotSpacing is the distance between the manifest slots, so a torn write of
// one slot never reaches the other.
const slotSpacing = 4096

// slotSize is the encoded length of a manifest slot.
const slotSize = 8*5 + 4*2

// recordHeader is the length of a log record header: the body length, the
// checksum, and the sequence.
const recordHeader = 4 + 4 + 8

// Log record operations.
const (
	// opSet sets a key.
	opSet byte = 1
	// opDelete deletes a key.
	opDelete byte = 2
)

// castagnoli is the CRC-32C table.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// manifest names the checkpoint and the log that together hold the index.
type manifest struct {
	// gen orders manifests; the slot with the higher valid gen wins.
	gen uint64
	// checkpoint is the checkpoint file number, zero for none.
	checkpoint uint64
	// checkpointLen is the checkpoint file length.
	checkpointLen uint64
	// checkpointSum is the CRC-32C of the checkpoint file.
	checkpointSum uint32
	// seq is the sequence of the last record the checkpoint holds.
	seq uint64
	// log is the first log file recovery replays after the checkpoint.
	log uint64
}

// marshal encodes the manifest into one slot.
func (m *manifest) marshal() []byte {
	b := make([]byte, 0, slotSize)
	b = binary.LittleEndian.AppendUint64(b, m.gen)
	b = binary.LittleEndian.AppendUint64(b, m.checkpoint)
	b = binary.LittleEndian.AppendUint64(b, m.checkpointLen)
	b = binary.LittleEndian.AppendUint32(b, m.checkpointSum)
	b = binary.LittleEndian.AppendUint64(b, m.seq)
	b = binary.LittleEndian.AppendUint64(b, m.log)
	return binary.LittleEndian.AppendUint32(b, crc32.Checksum(b, castagnoli))
}

// unmarshalManifest decodes a slot, reporting false for a torn or empty slot.
func unmarshalManifest(b []byte) (manifest, bool) {
	if len(b) < slotSize {
		return manifest{}, false
	}
	b = b[:slotSize]
	body, sum := b[:slotSize-4], binary.LittleEndian.Uint32(b[slotSize-4:])
	if crc32.Checksum(body, castagnoli) != sum {
		return manifest{}, false
	}
	m := manifest{
		gen:           binary.LittleEndian.Uint64(body[0:]),
		checkpoint:    binary.LittleEndian.Uint64(body[8:]),
		checkpointLen: binary.LittleEndian.Uint64(body[16:]),
		checkpointSum: binary.LittleEndian.Uint32(body[24:]),
		seq:           binary.LittleEndian.Uint64(body[28:]),
		log:           binary.LittleEndian.Uint64(body[36:]),
	}
	return m, m.gen != 0
}

// appendRecord appends a log record holding ops at seq.
func appendRecord(b []byte, seq uint64, ops []memtable.Op) []byte {
	start := len(b)
	b = append(b, make([]byte, recordHeader)...)
	for _, op := range ops {
		if op.Delete {
			b = append(b, opDelete)
			b = appendBytes(b, op.Key)
			continue
		}
		b = append(b, opSet)
		b = appendBytes(b, op.Key)
		b = appendBytes(b, op.Value)
	}
	header := b[start : start+recordHeader]
	binary.LittleEndian.PutUint32(header[0:], uint32(len(b)-start-recordHeader)) //nolint:gosec
	binary.LittleEndian.PutUint64(header[8:], seq)
	binary.LittleEndian.PutUint32(header[4:], crc32.Checksum(b[start+8:], castagnoli))
	return b
}

// readRecord decodes the record at the start of b, returning its sequence,
// its operations, and its length. It reports false for a torn, partial, or
// absent record.
func readRecord(b []byte) (uint64, []memtable.Op, int, bool) {
	if len(b) < recordHeader {
		return 0, nil, 0, false
	}
	n := int(binary.LittleEndian.Uint32(b[0:]))
	if n > len(b)-recordHeader {
		return 0, nil, 0, false
	}
	end := recordHeader + n
	if crc32.Checksum(b[8:end], castagnoli) != binary.LittleEndian.Uint32(b[4:]) {
		return 0, nil, 0, false
	}
	seq := binary.LittleEndian.Uint64(b[8:])
	var ops []memtable.Op
	for body := b[recordHeader:end]; len(body) != 0; {
		kind := body[0]
		key, rest, err := readBytes(body[1:])
		if err != nil {
			return 0, nil, 0, false
		}
		op := memtable.Op{Key: key, Delete: kind == opDelete}
		switch kind {
		case opSet:
			op.Value, rest, err = readBytes(rest)
			if err != nil {
				return 0, nil, 0, false
			}
		case opDelete:
		default:
			return 0, nil, 0, false
		}
		ops = append(ops, op)
		body = rest
	}
	return seq, ops, end, true
}

// appendBytes appends a length-prefixed byte string.
func appendBytes(b, v []byte) []byte {
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

// readBytes decodes a byte string written by appendBytes and returns the rest
// of b. The result aliases b.
func readBytes(b []byte) ([]byte, []byte, error) {
	n, size := binary.Uvarint(b)
	if size <= 0 || n > uint64(len(b)-size) { //nolint:gosec
		return nil, nil, errors.New("invalid index entry")
	}
	end := size + int(n) //nolint:gosec
	return b[size:end], b[end:], nil
}

// fileName returns the name of file n with prefix.
func fileName(prefix string, n uint64) string {
	return prefix + strconv.FormatUint(n, 10)
}

// parseFile returns the number of a file name with prefix.
func parseFile(prefix, name string) (uint64, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 10, 64)
	return n, err == nil
}
