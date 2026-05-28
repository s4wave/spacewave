package segment

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"slices"

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

// BuildResult describes a built SSTable and its point-lookup metadata.
type BuildResult struct {
	Written int64
	Lookup  *LookupMeta
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

// EstimatedSize returns a conservative byte-size hint for Build output.
func (w *Writer) EstimatedSize() int {
	if len(w.entries) == 0 {
		return 0
	}
	dataSize := 0
	indexSize := 0
	maxKeyLen := 0
	indexInterval := max(w.indexInterval, 1)
	for i := range w.entries {
		keyLen := len(w.entries[i].Key)
		maxKeyLen = max(maxKeyLen, keyLen)
		dataSize += EntryOverhead + keyLen
		if !w.entries[i].Tombstone {
			dataSize += len(w.entries[i].Value)
		}
		if i%indexInterval == 0 {
			indexSize += 2 + keyLen + 4
		}
	}
	bloomSize := len(NewBloomFilter(len(w.entries), w.bloomFPR).Encode())
	keyBlockSize := 2 + maxKeyLen + 2 + maxKeyLen
	return HeaderSize + keyBlockSize + dataSize + indexSize + bloomSize + 4
}

// Build sorts entries by key and writes the SSTable to dst.
// Returns the total bytes written.
func (w *Writer) Build(dst io.Writer) (int64, error) {
	result, err := w.BuildWithMeta(dst)
	if err != nil {
		return result.Written, err
	}
	return result.Written, nil
}

// BuildWithMeta sorts entries, streams the SSTable to dst, and returns lookup
// metadata for the written segment.
func (w *Writer) BuildWithMeta(dst io.Writer) (BuildResult, error) {
	if len(w.entries) == 0 {
		return BuildResult{}, errors.New("no entries")
	}

	slices.SortFunc(w.entries, func(a, b Entry) int {
		return bytes.Compare(a.Key, b.Key)
	})

	dataSize, indexEntries := w.planDataBlock()
	indexBlock := encodeIndex(indexEntries)

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
	dataOffset := mustUint32Len(HeaderSize + keyBlockSize)
	dataSizeUint32 := mustUint32Len(dataSize)
	indexOffset := dataOffset + dataSizeUint32
	indexSize := mustUint32Len(len(indexBlock))
	bloomOffset := indexOffset + indexSize
	bloomSize := mustUint32Len(len(bloomBlock))

	hdr := Header{
		Magic:       Magic,
		Version:     CurrentVersion,
		EntryCount:  mustUint32Len(len(w.entries)),
		DataOffset:  dataOffset,
		DataSize:    dataSizeUint32,
		IndexOffset: indexOffset,
		IndexSize:   indexSize,
		BloomOffset: bloomOffset,
		BloomSize:   bloomSize,
		MinKeySize:  mustUint16Len(len(minKey)),
		MaxKeySize:  mustUint16Len(len(maxKey)),
	}

	// Write everything into a CRC32 writer so we can compute the footer checksum.
	crc := crc32.NewIEEE()
	mw := io.MultiWriter(dst, crc)

	var total int64

	// Write header.
	var hdrBuf [HeaderSize]byte
	hdr.Encode(hdrBuf[:])
	n, err := writeFull(mw, hdrBuf[:])
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write header")
	}

	// Write min key.
	var lenBuf [4]byte
	binary.BigEndian.PutUint16(lenBuf[:2], mustUint16Len(len(minKey)))
	n, err = writeFull(mw, lenBuf[:2])
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write min key len")
	}
	n, err = writeFull(mw, minKey)
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write min key")
	}

	// Write max key.
	binary.BigEndian.PutUint16(lenBuf[:2], mustUint16Len(len(maxKey)))
	n, err = writeFull(mw, lenBuf[:2])
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write max key len")
	}
	n, err = writeFull(mw, maxKey)
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write max key")
	}

	// Write data block.
	n, err = w.writeDataBlock(mw)
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write data block")
	}

	// Write index block.
	if len(indexBlock) > 0 {
		n, err = writeFull(mw, indexBlock)
		total += n
		if err != nil {
			return BuildResult{Written: total}, errors.Wrap(err, "write index block")
		}
	}

	// Write bloom filter.
	if len(bloomBlock) > 0 {
		n, err = writeFull(mw, bloomBlock)
		total += n
		if err != nil {
			return BuildResult{Written: total}, errors.Wrap(err, "write bloom filter")
		}
	}

	// Write CRC32 footer (checksum of everything above).
	binary.BigEndian.PutUint32(lenBuf[:4], crc.Sum32())
	n, err = writeFull(dst, lenBuf[:4])
	total += n
	if err != nil {
		return BuildResult{Written: total}, errors.Wrap(err, "write footer")
	}

	return BuildResult{
		Written: total,
		Lookup: &LookupMeta{
			Header: &hdr,
			MinKey: cloneBytes(minKey),
			MaxKey: cloneBytes(maxKey),
			Index:  cloneIndexEntries(indexEntries),
			Bloom:  bf,
		},
	}, nil
}

func (w *Writer) planDataBlock() (int, []IndexEntry) {
	size := 0
	var index []IndexEntry
	off := 0
	indexInterval := max(w.indexInterval, 1)
	for i := range w.entries {
		if i%indexInterval == 0 {
			index = append(index, IndexEntry{
				Key:        w.entries[i].Key,
				DataOffset: mustUint32Len(off),
			})
		}
		entrySize := EntryOverhead + len(w.entries[i].Key)
		if !w.entries[i].Tombstone {
			entrySize += len(w.entries[i].Value)
		}
		size += entrySize
		off += entrySize
	}
	return size, index
}

func (w *Writer) writeDataBlock(dst io.Writer) (int64, error) {
	var total int64
	var lenBuf [4]byte
	for i := range w.entries {
		e := &w.entries[i]
		binary.BigEndian.PutUint16(lenBuf[:2], mustUint16Len(len(e.Key)))
		n, err := writeFull(dst, lenBuf[:2])
		total += n
		if err != nil {
			return total, errors.Wrap(err, "write key length")
		}
		n, err = writeFull(dst, e.Key)
		total += n
		if err != nil {
			return total, errors.Wrap(err, "write key")
		}
		valueLen := TombstoneLen
		if !e.Tombstone {
			valueLen = mustUint32Len(len(e.Value))
		}
		binary.BigEndian.PutUint32(lenBuf[:4], valueLen)
		n, err = writeFull(dst, lenBuf[:4])
		total += n
		if err != nil {
			return total, errors.Wrap(err, "write value length")
		}
		if !e.Tombstone {
			n, err = writeFull(dst, e.Value)
			total += n
			if err != nil {
				return total, errors.Wrap(err, "write value")
			}
		}
	}
	return total, nil
}

func writeFull(dst io.Writer, data []byte) (int64, error) {
	n, err := dst.Write(data)
	if n != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

func cloneBytes(data []byte) []byte {
	return bytes.Clone(data)
}

func cloneIndexEntries(entries []IndexEntry) []IndexEntry {
	out := make([]IndexEntry, len(entries))
	for i := range entries {
		out[i] = IndexEntry{
			Key:        cloneBytes(entries[i].Key),
			DataOffset: entries[i].DataOffset,
		}
	}
	return out
}
