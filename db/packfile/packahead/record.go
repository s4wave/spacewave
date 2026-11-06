package packahead

import (
	"encoding/binary"
	"hash/crc32"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
)

// recordMagic begins every batch record: "PAH1".
const recordMagic uint32 = 0x50414831

// recordHeaderLen is the bytes of magic and bodyLen.
const recordHeaderLen = 8

// recordFixedBody is the bytes of end, dataLen and dataCRC.
const recordFixedBody = 20

// recordTrailerLen is the bytes of recCRC.
const recordTrailerLen = 4

// crcTable is the CRC32C table shared by both record checksums.
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// errBadRecord ends a scan at a record that is torn, missing or not a record.
var errBadRecord = errors.New("bad batch record")

// Block is one value to append, under its pack index key.
type Block struct {
	// Key is the block's pack index key, at least keyTailLen bytes of a
	// uniformly distributed hash, such as packfile.BlockKey.
	Key []byte
	// Value is the encoded block object the pack stores.
	Value []byte
}

// entry is one value a pack holds, at its offset in the pack.
type entry struct {
	// key is the value's key. It may point into the pack's buffer, so it is
	// valid only until the pack is finalized.
	key []byte
	// off is the value's offset in the pack.
	off int64
	// size is the value's length.
	size int64
}

// recordLen returns the encoded length of the record for blocks.
func recordLen(blocks []Block) int {
	n := recordHeaderLen + recordFixedBody + recordTrailerLen
	n += protobuf_go_lite.SizeOfVarint(uint64(len(blocks)))
	for _, b := range blocks {
		n += protobuf_go_lite.SizeOfVarint(uint64(len(b.Key))) + len(b.Key)
		n += protobuf_go_lite.SizeOfVarint(uint64(len(b.Value)))
	}
	return n
}

// dataLen returns the summed length of the blocks' values.
func dataLen(blocks []Block) int {
	var n int
	for _, b := range blocks {
		n += len(b.Value)
	}
	return n
}

// encodeBatch writes the record for blocks and their values into dst, which
// begins at pack offset start and holds exactly recordLen + dataLen bytes. It
// returns the entries, whose keys point into dst.
func encodeBatch(dst []byte, start int64, blocks []Block) []entry {
	// Copy the values after the record and checksum them.
	recLen := recordLen(blocks)
	data := dst[recLen:]
	entries := make([]entry, len(blocks))
	var pos int
	for i, b := range blocks {
		copy(data[pos:], b.Value)
		entries[i] = entry{off: start + int64(recLen+pos), size: int64(len(b.Value))}
		pos += len(b.Value)
	}
	dataCRC := crc32.Checksum(data[:pos], crcTable)

	// Write the header and the fixed body.
	bodyLen := recLen - recordHeaderLen - recordTrailerLen
	binary.LittleEndian.PutUint32(dst[0:], recordMagic)
	binary.LittleEndian.PutUint32(dst[4:], uint32(bodyLen))                  //nolint:gosec // a record is bounded by the pack size.
	binary.LittleEndian.PutUint64(dst[8:], uint64(start)+uint64(recLen+pos)) //nolint:gosec // offsets are non-negative.
	binary.LittleEndian.PutUint64(dst[16:], uint64(pos))                     //nolint:gosec // lengths are non-negative.
	binary.LittleEndian.PutUint32(dst[24:], dataCRC)

	// Write the entries, pointing each key at its copy in the record.
	body := dst[:recordHeaderLen+recordFixedBody]
	body = protobuf_go_lite.AppendVarint(body, uint64(len(blocks)))
	for i, b := range blocks {
		body = protobuf_go_lite.AppendVarint(body, uint64(len(b.Key)))
		keyAt := len(body)
		body = append(body, b.Key...)
		entries[i].key = dst[keyAt:len(body):len(body)]
		body = protobuf_go_lite.AppendVarint(body, uint64(len(b.Value)))
	}

	// Seal the record with its own checksum.
	binary.LittleEndian.PutUint32(dst[len(body):], crc32.Checksum(body, crcTable))
	return entries
}

// decodeBatch reads the batch at offset start of pack, the pack's bytes as far
// as they reach. It returns the batch's entries, whose keys point into pack,
// and the offset after its values. errBadRecord reports a record that is
// incomplete or fails either checksum.
func decodeBatch(pack []byte, start int64) ([]entry, int64, error) {
	// Read the header and bound the record by the bytes present.
	rec := pack[start:]
	if len(rec) < recordHeaderLen+recordFixedBody+recordTrailerLen {
		return nil, 0, errBadRecord
	}
	if binary.LittleEndian.Uint32(rec) != recordMagic {
		return nil, 0, errBadRecord
	}
	bodyLen := int(binary.LittleEndian.Uint32(rec[4:]))
	recLen := recordHeaderLen + bodyLen + recordTrailerLen
	if bodyLen < recordFixedBody || recLen > len(rec) {
		return nil, 0, errBadRecord
	}

	// Check the record's own checksum before trusting its fields.
	sealed := rec[:recordHeaderLen+bodyLen]
	if crc32.Checksum(sealed, crcTable) != binary.LittleEndian.Uint32(rec[len(sealed):]) {
		return nil, 0, errBadRecord
	}
	end := binary.LittleEndian.Uint64(rec[8:])
	n := binary.LittleEndian.Uint64(rec[16:])
	dataCRC := binary.LittleEndian.Uint32(rec[24:])
	if n > uint64(len(rec)-recLen) || end != uint64(start)+uint64(recLen)+n { //nolint:gosec // offsets are non-negative.
		return nil, 0, errBadRecord
	}

	// Check the values against the record's checksum of them.
	data := rec[recLen : recLen+int(n)] //nolint:gosec // n is bounded by the bytes present.
	if crc32.Checksum(data, crcTable) != dataCRC {
		return nil, 0, errBadRecord
	}

	// Parse the entries and place each value.
	body := rec[recordHeaderLen+recordFixedBody : recordHeaderLen+bodyLen]
	count, body, err := readVarint(body)
	if err != nil || count > uint64(len(body)) {
		return nil, 0, errBadRecord
	}
	entries := make([]entry, 0, count)
	off := start + int64(recLen)
	for range count {
		var keyLen, size uint64
		keyLen, body, err = readVarint(body)
		if err != nil || keyLen > uint64(len(body)) {
			return nil, 0, errBadRecord
		}
		key := body[:keyLen:keyLen]
		size, body, err = readVarint(body[keyLen:])
		if err != nil || off+int64(size) > int64(end) { //nolint:gosec // bounded by end.
			return nil, 0, errBadRecord
		}
		entries = append(entries, entry{key: key, off: off, size: int64(size)}) //nolint:gosec // bounded by end.
		off += int64(size)                                                      //nolint:gosec // bounded by end.
	}
	if len(body) != 0 || off != int64(end) { //nolint:gosec // end is bounded by the pack.
		return nil, 0, errBadRecord
	}
	return entries, int64(end), nil //nolint:gosec // end is bounded by the pack.
}

// scanBatches decodes the batches of pack from its start until the first bad
// record, returning their entries and the offset where the good batches end.
func scanBatches(pack []byte) ([]entry, int64) {
	var entries []entry
	var at int64
	for at < int64(len(pack)) {
		batch, end, err := decodeBatch(pack, at)
		if err != nil {
			break
		}
		entries = append(entries, batch...)
		at = end
	}
	return entries, at
}

// readVarint reads one uvarint from the front of b.
func readVarint(b []byte) (uint64, []byte, error) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, nil, errBadRecord
	}
	return v, b[n:], nil
}
