package packahead

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"

	"github.com/pkg/errors"
)

// scanChunk bounds one read of a batch's values while ScanRecords checks
// their checksum.
const scanChunk = 1 << 20

// ScanRecords calls fn with each value of the leading valid batches of the
// pack of size bytes in ra, in write order, and returns the offset where the
// valid batches end. It is the read-only form of the recovery Open runs: it
// never truncates, finalizes or removes the pack, so the bytes past that
// offset stay for the caller to report. It holds one record of at most
// maxBytes and one value at a time, and checks a batch's values against the
// record's checksum of them before passing any to fn. A finalized pack keeps
// its batch records, so its values read this way too. fn must copy what it
// keeps.
func ScanRecords(ctx context.Context, ra io.ReaderAt, size, maxBytes int64, fn func(key, value []byte) error) (int64, error) {
	var at int64
	for size-at >= recordHeaderLen+recordFixedBody+recordTrailerLen {
		if err := ctx.Err(); err != nil {
			return at, err
		}

		// Read the header, then the whole record it bounds.
		var header [recordHeaderLen]byte
		if _, err := ra.ReadAt(header[:], at); err != nil {
			return at, errors.Wrap(err, "read batch record")
		}
		if !bytes.HasPrefix(header[:], []byte(recordMagic)) {
			break
		}
		recLen := int64(recordHeaderLen) + int64(binary.LittleEndian.Uint32(header[4:])) + recordTrailerLen
		if recLen > size-at || recLen > maxBytes {
			break
		}
		rec := make([]byte, recLen)
		if _, err := ra.ReadAt(rec, at); err != nil {
			return at, errors.Wrap(err, "read batch record")
		}
		entries, _, n, dataCRC, end, err := parseRecord(rec, at, size-at)
		if err == errSkippedRecord {
			// A recovery skip record seals a damaged extent that holds no
			// values; the batches past it are still the pack's.
			at = end
			continue
		}
		if err != nil {
			break
		}

		// Check the values against the record before trusting any of them.
		sum, err := checksumRange(ra, at+recLen, int64(n))
		if err != nil {
			return at, err
		}
		if sum != dataCRC {
			break
		}

		// Pass each value, read on its own.
		for _, e := range entries {
			if e.size > maxBytes {
				return at, errors.Errorf("batch value of %d bytes exceeds %d", e.size, maxBytes)
			}
			value := make([]byte, e.size)
			if _, err := ra.ReadAt(value, e.off); err != nil {
				return at, errors.Wrap(err, "read batch value")
			}
			if err := fn(e.key, value); err != nil {
				return at, err
			}
		}
		at = end
	}
	return at, nil
}

// checksumRange returns the CRC32C of n bytes of ra at off, read in chunks of
// at most scanChunk.
func checksumRange(ra io.ReaderAt, off, n int64) (uint32, error) {
	buf := make([]byte, min(n, scanChunk))
	var sum uint32
	for n > 0 {
		part := buf[:min(n, int64(len(buf)))]
		if _, err := ra.ReadAt(part, off); err != nil {
			return 0, errors.Wrap(err, "read batch values")
		}
		sum = crc32.Update(sum, crcTable, part)
		off += int64(len(part))
		n -= int64(len(part))
	}
	return sum, nil
}

// ClassifyName reports whether name is the file of an open pack log or of a
// finalized pack. A name that is neither is no pack of a Log.
func ClassifyName(name string) (open, finalized bool) {
	_, ext, _, valid := parseName(name)
	return valid && ext == openExt, valid && ext == packExt
}
