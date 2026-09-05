package segment

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// EntryIterator streams entries from an SSTable data block in key order.
// Its bounded read buffer keeps scans sequential even when a shared cache is full.
type EntryIterator struct {
	r         io.Reader
	off       uint32
	limit     uint32
	remaining uint32
}

// NewEntryIterator returns an iterator over the entries described by metadata.
func NewEntryIterator(r io.ReaderAt, meta *LookupMeta) *EntryIterator {
	reader := io.NewSectionReader(r, int64(meta.Header.DataOffset), int64(meta.Header.DataSize))
	return &EntryIterator{
		r:         bufio.NewReaderSize(reader, 64*1024),
		limit:     meta.Header.DataSize,
		remaining: meta.Header.EntryCount,
	}
}

// Entries returns a streaming iterator over the reader's data block.
func (rd *Reader) Entries() *EntryIterator {
	reader := io.NewSectionReader(rd.r, int64(rd.header.DataOffset), int64(rd.header.DataSize))
	return &EntryIterator{
		r:         bufio.NewReaderSize(reader, 64*1024),
		limit:     rd.header.DataSize,
		remaining: rd.header.EntryCount,
	}
}

// Next returns the next entry, or ok=false when the iterator is exhausted.
func (it *EntryIterator) Next() (entry Entry, ok bool, err error) {
	if it.remaining == 0 {
		return Entry{}, false, nil
	}
	if it.off >= it.limit {
		return Entry{}, false, errors.New("truncated entry: data block exhausted")
	}

	var hdr [4]byte
	if err := it.read(hdr[:2]); err != nil {
		return Entry{}, false, errors.Wrap(err, "read key length")
	}
	keyLen := uint32(binary.BigEndian.Uint16(hdr[:2]))
	keyLenInt, err := uint32ToInt(keyLen)
	if err != nil {
		return Entry{}, false, err
	}
	key := make([]byte, keyLenInt)
	if keyLenInt != 0 {
		if err := it.read(key); err != nil {
			return Entry{}, false, errors.Wrap(err, "read key")
		}
	}

	if err := it.read(hdr[:4]); err != nil {
		return Entry{}, false, errors.Wrap(err, "read value length")
	}
	valLen := binary.BigEndian.Uint32(hdr[:4])
	it.remaining--
	if valLen == TombstoneLen {
		return Entry{Key: key, Tombstone: true}, true, nil
	}
	if it.off > it.limit || it.limit-it.off < valLen {
		return Entry{}, false, io.ErrUnexpectedEOF
	}

	valLenInt, err := uint32ToInt(valLen)
	if err != nil {
		return Entry{}, false, err
	}
	value := make([]byte, valLenInt)
	if valLenInt != 0 {
		if err := it.read(value); err != nil {
			return Entry{}, false, errors.Wrap(err, "read value")
		}
	}
	return Entry{Key: key, Value: value}, true, nil
}

func (it *EntryIterator) read(buf []byte) error {
	if it.off > it.limit {
		return io.ErrUnexpectedEOF
	}
	if uint64(it.limit-it.off) < uint64(len(buf)) {
		return io.ErrUnexpectedEOF
	}
	if _, err := io.ReadFull(it.r, buf); err != nil {
		return err
	}
	it.off += uint32(len(buf))
	return nil
}
