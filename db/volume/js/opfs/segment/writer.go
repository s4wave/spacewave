package segment

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"sort"

	"github.com/pkg/errors"
)

// DefaultBloomFPR is the default false-positive rate for bloom filters (0.1%).
const DefaultBloomFPR = 0.001

// Writer builds an SSTable from a set of entries and writes it to an io.Writer.
// Entries are sorted by key during Build.
type Writer struct {
	entries       []Entry
	indexInterval int
	bloomFPR      float64
}

// NewWriter creates a new SSTable writer with default settings.
func NewWriter() *Writer {
	return &Writer{
		indexInterval: DefaultIndexInterval,
		bloomFPR:      DefaultBloomFPR,
	}
}

// SetIndexInterval sets the number of entries between sparse index entries.
// Must be called before Build.
func (w *Writer) SetIndexInterval(n int) {
	if n < 1 {
		n = 1
	}
	w.indexInterval = n
}

// SetBloomFPR sets the target false-positive rate for the bloom filter.
// Must be called before Build.
func (w *Writer) SetBloomFPR(fpr float64) {
	w.bloomFPR = fpr
}

// Add appends an entry to the writer. Entries need not be pre-sorted.
func (w *Writer) Add(key, value []byte) {
	w.entries = append(w.entries, Entry{Key: key, Value: value})
}

// AddTombstone appends a deletion marker for the given key.
func (w *Writer) AddTombstone(key []byte) {
	w.entries = append(w.entries, Entry{Key: key, Tombstone: true})
}

// Reset clears all entries so the writer can be reused.
func (w *Writer) Reset() {
	w.entries = w.entries[:0]
}

// Build sorts entries by key and writes the SSTable to dst.
// Returns the total bytes written.
func (w *Writer) Build(dst io.Writer) (int64, error) {
	if len(w.entries) == 0 {
		return 0, errors.New("no entries")
	}

	sort.Slice(w.entries, func(i, j int) bool {
		return string(w.entries[i].Key) < string(w.entries[j].Key)
	})

	// Encode the data block and build sparse index simultaneously.
	dataBlock, indexEntries := w.encodeDataBlockWithIndex()
	indexBlock := encodeIndex(indexEntries)

	// Build bloom filter.
	bf := NewBloomFilter(len(w.entries), w.bloomFPR)
	for i := range w.entries {
		bf.Add(w.entries[i].Key)
	}
	bloomBlock := bf.Encode()

	minKey := w.entries[0].Key
	maxKey := w.entries[len(w.entries)-1].Key

	// Compute layout offsets.
	// After the fixed header, we store min key and max key with u16 length prefixes.
	keyBlockSize := 2 + len(minKey) + 2 + len(maxKey)
	dataOffset := uint32(HeaderSize + keyBlockSize)
	dataSize := uint32(len(dataBlock))
	indexOffset := dataOffset + dataSize
	indexSize := uint32(len(indexBlock))
	bloomOffset := indexOffset + indexSize
	bloomSize := uint32(len(bloomBlock))

	hdr := Header{
		Magic:       Magic,
		Version:     CurrentVersion,
		EntryCount:  uint32(len(w.entries)),
		DataOffset:  dataOffset,
		DataSize:    dataSize,
		IndexOffset: indexOffset,
		IndexSize:   indexSize,
		BloomOffset: bloomOffset,
		BloomSize:   bloomSize,
		MinKeySize:  uint16(len(minKey)),
		MaxKeySize:  uint16(len(maxKey)),
	}

	// Write everything into a CRC32 writer so we can compute the footer checksum.
	crc := crc32.NewIEEE()
	mw := io.MultiWriter(dst, crc)

	var total int64

	// Write header.
	var hdrBuf [HeaderSize]byte
	hdr.Encode(hdrBuf[:])
	n, err := mw.Write(hdrBuf[:])
	if err != nil {
		return total, errors.Wrap(err, "write header")
	}
	total += int64(n)

	// Write min key.
	var lenBuf [4]byte
	binary.BigEndian.PutUint16(lenBuf[:2], uint16(len(minKey)))
	n, err = mw.Write(lenBuf[:2])
	if err != nil {
		return total, errors.Wrap(err, "write min key len")
	}
	total += int64(n)
	n, err = mw.Write(minKey)
	if err != nil {
		return total, errors.Wrap(err, "write min key")
	}
	total += int64(n)

	// Write max key.
	binary.BigEndian.PutUint16(lenBuf[:2], uint16(len(maxKey)))
	n, err = mw.Write(lenBuf[:2])
	if err != nil {
		return total, errors.Wrap(err, "write max key len")
	}
	total += int64(n)
	n, err = mw.Write(maxKey)
	if err != nil {
		return total, errors.Wrap(err, "write max key")
	}
	total += int64(n)

	// Write data block.
	n, err = mw.Write(dataBlock)
	if err != nil {
		return total, errors.Wrap(err, "write data block")
	}
	total += int64(n)

	// Write index block.
	if len(indexBlock) > 0 {
		n, err = mw.Write(indexBlock)
		if err != nil {
			return total, errors.Wrap(err, "write index block")
		}
		total += int64(n)
	}

	// Write bloom filter.
	if len(bloomBlock) > 0 {
		n, err = mw.Write(bloomBlock)
		if err != nil {
			return total, errors.Wrap(err, "write bloom filter")
		}
		total += int64(n)
	}

	// Write CRC32 footer (checksum of everything above).
	binary.BigEndian.PutUint32(lenBuf[:4], crc.Sum32())
	n, err = dst.Write(lenBuf[:4])
	if err != nil {
		return total, errors.Wrap(err, "write footer")
	}
	total += int64(n)

	return total, nil
}

// encodeDataBlockWithIndex serializes sorted entries and builds sparse index entries.
// An index entry is emitted every w.indexInterval entries.
func (w *Writer) encodeDataBlockWithIndex() ([]byte, []IndexEntry) {
	size := 0
	for i := range w.entries {
		size += EntryOverhead + len(w.entries[i].Key)
		if !w.entries[i].Tombstone {
			size += len(w.entries[i].Value)
		}
	}

	buf := make([]byte, size)
	var index []IndexEntry
	off := 0
	for i := range w.entries {
		// Emit an index entry at the start and every Nth entry.
		if i%w.indexInterval == 0 {
			index = append(index, IndexEntry{
				Key:        w.entries[i].Key,
				DataOffset: uint32(off),
			})
		}

		e := &w.entries[i]
		binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(e.Key)))
		off += 2
		copy(buf[off:], e.Key)
		off += len(e.Key)
		if e.Tombstone {
			binary.BigEndian.PutUint32(buf[off:off+4], TombstoneLen)
		} else {
			binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(e.Value)))
		}
		off += 4
		if !e.Tombstone {
			copy(buf[off:], e.Value)
			off += len(e.Value)
		}
	}
	return buf, index
}
