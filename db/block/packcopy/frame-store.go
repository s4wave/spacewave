package packcopy

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"os"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// FrameStore is a source over block frame files: the blocks a live store
// exported for a list of holes. A frame is the uvarint length of the marshaled
// hash, the hash, a uvarint that is 1 when the refs are known, the uvarint
// length of the encoded block.BlockObject, and the object.
type FrameStore struct {
	// files are the open frame files.
	files []*os.File
	// frames places each block by pack index key. The last frame wins.
	frames map[string]framePlace
}

// framePlace is where one block's object sits.
type framePlace struct {
	// file holds the object.
	file *os.File
	// offset is where the object starts.
	offset int64
	// size is the object's length.
	size int
	// known is set when the block's refs are known.
	known bool
}

// OpenFrameStore scans the frame files and keeps them open. A file cut short,
// as by a stopped export, yields the frames before the cut.
func OpenFrameStore(paths []string) (*FrameStore, error) {
	s := &FrameStore{frames: make(map[string]framePlace)}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.files = append(s.files, f)
		if err := s.scan(f); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

// scan indexes one file, skipping each object.
func (s *FrameStore) scan(f *os.File) error {
	r := &countReader{r: bufio.NewReaderSize(f, 1<<20)}
	for {
		// Read the header. A short read is the cut tail.
		key, known, size, err := readFrameHeader(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Skip the object, keeping it only when it is whole.
		offset := r.n
		skipped, err := r.r.Discard(size)
		r.n += int64(skipped)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.frames[string(key)] = framePlace{file: f, offset: offset, size: size, known: known}
	}
}

// readFrameHeader reads a frame up to its object.
func readFrameHeader(r *countReader) (key []byte, known bool, size int, err error) {
	// Read the hash.
	keyLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, false, 0, err
	}
	key = make([]byte, keyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, false, 0, io.ErrUnexpectedEOF
	}

	// Read the flag and the object's length.
	flag, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, false, 0, io.ErrUnexpectedEOF
	}
	objLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, false, 0, io.ErrUnexpectedEOF
	}
	return key, flag == 1, int(objLen), nil //nolint:gosec // a frame holds one block, far under the int range.
}

// GetStoredBlock reads the block's frame, or returns nil.
func (s *FrameStore) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Find and read the object.
	place, ok := s.frames[string(packfile.BlockKey(ref.GetHash()))]
	if !ok {
		return nil, nil
	}
	obj := make([]byte, place.size)
	if _, err := place.file.ReadAt(obj, place.offset); err != nil {
		return nil, err
	}

	// Decode it, taking the frame's word on its refs.
	stored, err := block.DecodeBlockObject(obj)
	if err != nil {
		return nil, err
	}
	stored.RefsKnown = place.known
	return stored, nil
}

// Close closes the files.
func (s *FrameStore) Close() {
	for _, f := range s.files {
		_ = f.Close()
	}
}

// countReader tracks the offset of a buffered read.
type countReader struct {
	// r reads the file.
	r *bufio.Reader
	// n is the bytes consumed.
	n int64
}

// Read reads and counts.
func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// ReadByte reads one byte and counts it.
func (c *countReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// _ is a type assertion.
var _ Source = (*FrameStore)(nil)
