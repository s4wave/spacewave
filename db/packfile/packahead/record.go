package packahead

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// recordMagic is the "PAH1" marker in its on-disk little-endian byte order.
const recordMagic = "1HAP"

// recordHeaderLen is the bytes of magic and bodyLen.
const recordHeaderLen = 8

// recordFixedBody is the bytes of end, dataLen and dataCRC.
const recordFixedBody = 20

// recordTrailerLen is the bytes of recCRC.
const recordTrailerLen = 4

// skipRecordLen is the encoded length of the record that seals a damaged gap.
const skipRecordLen = recordHeaderLen + recordFixedBody + 1 + recordTrailerLen

// crcTable is the CRC32C table shared by both record checksums.
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// errBadRecord marks a record that is torn, corrupt or missing.
var errBadRecord = errors.New("bad batch record")

// errSkippedRecord marks a damaged batch with a repaired boundary.
var errSkippedRecord = errors.New("skipped batch record")

// Block is one value to append, under its pack index key.
type Block struct {
	// Key is the block's pack index key, at least a digest tail long, with a
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
	// batch is the offset of the record describing this value.
	batch int64
	// stripe fences decoded reads when the log loses this location.
	stripe block.BlockRemovalStripe
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
		entries[i] = entry{batch: start, off: start + int64(recLen+pos), size: int64(len(b.Value))}
		pos += len(b.Value)
	}
	dataCRC := crc32.Checksum(data[:pos], crcTable)

	// Write the header and the fixed body.
	bodyLen := recLen - recordHeaderLen - recordTrailerLen
	encodeRecordHeader(dst, bodyLen, start+int64(recLen+pos), int64(pos), dataCRC)

	// Write the entries, pointing each key at its copy in the record.
	body := dst[:recordHeaderLen+recordFixedBody]
	body = protobuf_go_lite.AppendVarint(body, uint64(len(blocks)))
	for i, b := range blocks {
		body = protobuf_go_lite.AppendVarint(body, uint64(len(b.Key)))
		keyAt := len(body)
		body = append(body, b.Key...)
		entries[i].key = dst[keyAt:len(body):len(body)]
		entries[i].stripe = block.BlockRemovalStripeForDigest(b.Key)
		body = protobuf_go_lite.AppendVarint(body, uint64(len(b.Value)))
	}

	// Seal the record with its own checksum.
	binary.LittleEndian.PutUint32(dst[len(body):], crc32.Checksum(body, crcTable))
	return entries
}

// encodeRecordHeader writes the record's framing and fixed fields.
func encodeRecordHeader(dst []byte, bodyLen int, end, dataLen int64, dataCRC uint32) {
	// Describe the extent and payload before the caller seals the record.
	copy(dst, recordMagic)
	binary.LittleEndian.PutUint32(dst[4:], uint32(bodyLen))  //nolint:gosec // bounded by the pack size.
	binary.LittleEndian.PutUint64(dst[8:], uint64(end))      //nolint:gosec // offsets are non-negative.
	binary.LittleEndian.PutUint64(dst[16:], uint64(dataLen)) //nolint:gosec // lengths are non-negative.
	binary.LittleEndian.PutUint32(dst[24:], dataCRC)
}

// encodeSkipRecord seals a known extent without admitting any values. The
// batch must be large enough for the zero-entry record and its skipped bytes.
func encodeSkipRecord(start, end int64) [skipRecordLen]byte {
	// Checksum the boundary and zero entry count without reading the gap.
	var rec [skipRecordLen]byte
	encodeRecordHeader(rec[:], recordFixedBody+1, end, end-start-skipRecordLen, 0)
	binary.LittleEndian.PutUint32(rec[skipRecordLen-recordTrailerLen:], crc32.Checksum(rec[:skipRecordLen-recordTrailerLen], crcTable))
	return rec
}

// decodeBatch reads the batch at offset start of pack, the pack's bytes as far
// as they reach. It returns the batch's entries, whose keys point into pack,
// and the offset after its values. On a bad record, a nonzero end means
// the record checksum still verified the extent to skip.
func decodeBatch(pack []byte, start int64) ([]entry, int64, error) {
	// Read the header and bound the record by the bytes present.
	rec := pack[start:]
	if len(rec) < recordHeaderLen+recordFixedBody+recordTrailerLen {
		return nil, 0, errBadRecord
	}
	if !bytes.HasPrefix(rec, []byte(recordMagic)) {
		return nil, 0, errBadRecord
	}
	bodySize := uint64(binary.LittleEndian.Uint32(rec[4:]))
	if bodySize < recordFixedBody || bodySize > uint64(len(rec)-recordHeaderLen-recordTrailerLen) { //nolint:gosec // minimum record length checked above.
		return nil, 0, errBadRecord
	}
	bodyLen := int(bodySize) //nolint:gosec // bounded by the slice length.
	recLen := recordHeaderLen + bodyLen + recordTrailerLen

	// Trust the stored end once the record checksum and extent verify.
	sealed := rec[:recordHeaderLen+bodyLen]
	end := binary.LittleEndian.Uint64(rec[8:])
	n := binary.LittleEndian.Uint64(rec[16:])
	dataCRC := binary.LittleEndian.Uint32(rec[24:])
	if n > uint64(len(rec)-recLen) || end != uint64(start)+uint64(recLen)+n || //nolint:gosec // offsets are non-negative.
		crc32.Checksum(sealed, crcTable) != binary.LittleEndian.Uint32(rec[len(sealed):]) {
		return nil, 0, errBadRecord
	}

	// A recovery record names a damaged extent and carries no values.
	if bodyLen == recordFixedBody+1 && rec[recordHeaderLen+recordFixedBody] == 0 {
		return nil, int64(end), errSkippedRecord //nolint:gosec // end is bounded by the pack.
	}

	// Check the values against the record's checksum of them.
	data := rec[recLen : recLen+int(n)] //nolint:gosec // n is bounded by the bytes present.
	if crc32.Checksum(data, crcTable) != dataCRC {
		return nil, int64(end), errBadRecord //nolint:gosec // end is bounded by the pack.
	}

	// Parse the entries and place each value.
	body := rec[recordHeaderLen+recordFixedBody : recordHeaderLen+bodyLen]
	count, body, err := readVarint(body)
	if err != nil || count > uint64(len(body)) {
		return nil, int64(end), errBadRecord //nolint:gosec // end is bounded by the pack.
	}
	entries := make([]entry, 0, count)
	off := start + int64(recLen)
	for range count {
		var keyLen, size uint64
		keyLen, body, err = readVarint(body)
		if err != nil || keyLen < block.BlockDigestTailLen || keyLen > uint64(len(body)) {
			return nil, int64(end), errBadRecord //nolint:gosec // end is bounded by the pack.
		}
		key := body[:keyLen:keyLen]
		size, body, err = readVarint(body[keyLen:])
		if err != nil || size > end-uint64(off) { //nolint:gosec // offsets are non-negative.
			return nil, int64(end), errBadRecord //nolint:gosec // end is bounded by the pack.
		}
		entries = append(entries, entry{key: key, batch: start, off: off, size: int64(size), stripe: block.BlockRemovalStripeForDigest(key)}) //nolint:gosec // bounded by end.
		off += int64(size)                                                                                                                    //nolint:gosec // bounded by end.
	}
	if len(body) != 0 || off != int64(end) { //nolint:gosec // end is bounded by the pack.
		return nil, int64(end), errBadRecord //nolint:gosec // end is bounded by the pack.
	}
	return entries, int64(end), nil //nolint:gosec // end is bounded by the pack.
}

// scanBatches recovers batches at known offsets on startup. A verified record
// gives the next boundary even when its values fail their checksum. Live
// recovery seals unreadable gaps before publishing its result. A header
// without a verified boundary stops the scan.
func scanBatches(pack []byte) (entries []entry, keptEnd int64, damaged bool) {
	// Decode only at the start of the file and the ends of verified records.
	for at := int64(0); at < int64(len(pack)); {
		batch, end, err := decodeBatch(pack, at)
		if err == nil {
			entries = append(entries, batch...)
			keptEnd = end
		}
		damaged = damaged || err != nil
		if end == 0 {
			break
		}
		at = end
	}
	return entries, keptEnd, damaged
}

// scanWrittenBatches uses the writer's original batch boundaries and bytes
// to verify each complete batch at its original offset during live recovery.
func scanWrittenBatches(disk, written []byte, original []entry) (entries []entry, keptEnd int64, damaged bool) {
	// Compare each written extent with its on-disk bytes. The original entries
	// may already have gaps if an earlier recovery attempt kept only part.
	for len(original) != 0 {
		n, start, end := firstBatchExtent(original)
		if end <= int64(len(disk)) && bytes.Equal(disk[start:end], written[start:end]) {
			entries = append(entries, original[:n]...)
			keptEnd = end
		} else {
			damaged = true
		}
		original = original[n:]
	}
	return entries, keptEnd, damaged || keptEnd < int64(len(disk))
}

// firstBatchExtent gives the entry count and byte span of the first batch.
func firstBatchExtent(entries []entry) (n int, start, end int64) {
	// Every entry in the batch shares its record offset.
	start = entries[0].batch
	n = 1
	for n < len(entries) && entries[n].batch == start {
		n++
	}

	// Values follow the record in the same order as its entries.
	last := entries[n-1]
	return n, start, last.off + last.size
}

// readVarint reads one uvarint from the front of b.
func readVarint(b []byte) (uint64, []byte, error) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, nil, errBadRecord
	}
	return v, b[n:], nil
}
