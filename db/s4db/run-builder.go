package s4db

import (
	"bytes"
	"encoding/binary"
	"slices"

	"github.com/pkg/errors"
)

// partitionBlocks is the data blocks one index partition and one filter
// partition cover.
const partitionBlocks = 256

// footerMagic ends a run footer.
var footerMagic = [4]byte{'S', '4', 'R', 'N'}

// runBuilder writes one run into reserved chunks: data blocks, then an index
// partition and a filter partition per partitionBlocks blocks, then the
// footer. No block straddles two chunks, so the chunks need not be adjacent.
type runBuilder struct {
	// s is the file.
	s storage
	// chunks are the chunks to write, in order; more returns another when
	// they run out.
	chunks []uint64
	more   func() (uint64, error)
	// used counts the chunks written or being written.
	used int
	// buf holds the current chunk's bytes.
	buf []byte
	// written counts the bytes written to the file.
	written int64

	// bb builds the current data block, and blockFirst is its first key.
	bb         blockBuilder
	blockFirst []byte
	// fences holds the current partition's fences: a block's first key and
	// location.
	fences []fence
	// hashes holds the current partition's key hashes for its filter.
	hashes []uint64
	// parts holds the finished partitions.
	parts []partition
	// count counts the entries, and smallest and largest bound the keys.
	count             uint64
	smallest, largest []byte
}

// fence locates one data block by its first key.
type fence struct {
	// key is the block's first key.
	key []byte
	// at locates the block.
	at ref
}

// partition is one top index entry: the first key of its blocks and the
// locations of its index and filter partitions.
type partition struct {
	// key is the first key of the partition's first block.
	key []byte
	// index and filter locate the partition's encoded fences and filter.
	index, filter ref
}

// newRunBuilder returns a builder writing into chunks, asking more for
// another chunk when they run out.
func newRunBuilder(s storage, chunks []uint64, more func() (uint64, error)) *runBuilder {
	return &runBuilder{s: s, chunks: chunks, more: more, buf: make([]byte, 0, chunkSize)}
}

// add appends key with value v. Keys arrive in ascending order; the first
// key out of order fails with ErrOutOfOrder.
func (rb *runBuilder) add(key []byte, v value) error {
	// Check the order and start a block when needed.
	if rb.count != 0 && bytes.Compare(key, rb.largest) <= 0 {
		return ErrOutOfOrder
	}
	if rb.bb.entries == 0 {
		rb.blockFirst = bytes.Clone(key)
	}
	if rb.count == 0 {
		rb.smallest = bytes.Clone(key)
	}
	rb.largest = append(rb.largest[:0], key...)

	// Add the entry, closing the block when it is full.
	rb.bb.add(key, v)
	rb.hashes = append(rb.hashes, keyHash(key))
	rb.count++
	if rb.bb.size() >= blockSize {
		return rb.closeBlock()
	}
	return nil
}

// closeBlock writes the current block and records its fence, closing the
// partition when it is full.
func (rb *runBuilder) closeBlock() error {
	// Write the block.
	if rb.bb.entries == 0 {
		return nil
	}
	b := rb.bb.finish()
	at, err := rb.place(b)
	if err != nil {
		return err
	}
	rb.fences = append(rb.fences, fence{key: rb.blockFirst, at: at})

	// Close the partition when it holds enough blocks.
	if len(rb.fences) >= partitionBlocks {
		return rb.closePartition()
	}
	return nil
}

// closePartition writes the current partition's index and filter.
func (rb *runBuilder) closePartition() error {
	// Encode and place the fences.
	if len(rb.fences) == 0 {
		return nil
	}
	ib := make([]byte, 0, 64*len(rb.fences))
	ib = binary.AppendUvarint(ib, uint64(len(rb.fences)))
	for _, f := range rb.fences {
		ib = binary.AppendUvarint(ib, uint64(len(f.key)))
		ib = append(ib, f.key...)
		ib = appendRef(ib, f.at)
	}
	indexAt, err := rb.place(ib)
	if err != nil {
		return err
	}

	// Build and place the filter.
	f := newRunFilter(len(rb.hashes))
	for _, h := range rb.hashes {
		f.addHash(h)
	}
	filterAt, err := rb.place(f.encode())
	if err != nil {
		return err
	}

	// Record the partition and reset.
	rb.parts = append(rb.parts, partition{key: rb.fences[0].key, index: indexAt, filter: filterAt})
	rb.fences, rb.hashes = nil, rb.hashes[:0]
	return nil
}

// place writes b whole into the current chunk, or the next when it does not
// fit, and returns its location.
func (rb *runBuilder) place(b []byte) (ref, error) {
	// Move to the next chunk when b does not fit; a block is far smaller
	// than a chunk.
	if len(rb.buf)+len(b) > chunkSize {
		if err := rb.flushChunk(); err != nil {
			return ref{}, err
		}
	}
	if len(rb.buf) == 0 && rb.used == len(rb.chunks) {
		c, err := rb.more()
		if err != nil {
			return ref{}, err
		}
		rb.chunks = append(rb.chunks, c)
	}

	// Append to the chunk buffer.
	off := chunkStart(rb.chunks[rb.used]) + uint64(len(rb.buf))
	rb.buf = append(rb.buf, b...)
	return ref{off: off, n: uint32(len(b)), crc: checksum(b)}, nil // #nosec G115 -- a block fits a chunk.
}

// flushChunk writes the current chunk buffer and moves to the next chunk.
func (rb *runBuilder) flushChunk() error {
	// Write the buffer at its chunk and advance.
	if len(rb.buf) == 0 {
		return nil
	}
	if _, err := rb.s.WriteAt(rb.buf, fileOff(chunkStart(rb.chunks[rb.used]))); err != nil {
		return err
	}
	rb.written += int64(len(rb.buf))
	rb.used++
	rb.buf = rb.buf[:0]
	return nil
}

// finish writes the last block, partition and the footer, flushes the last
// chunk, and returns the run's description for the record, with id and level
// filled by the caller.
func (rb *runBuilder) finish() (runMeta, error) {
	// Close what is open.
	if err := rb.closeBlock(); err != nil {
		return runMeta{}, err
	}
	if err := rb.closePartition(); err != nil {
		return runMeta{}, err
	}

	// Encode the footer's count and bounds.
	fb := make([]byte, 0, 64+len(rb.parts)*96)
	fb = binary.AppendUvarint(fb, rb.count)
	fb = binary.AppendUvarint(fb, uint64(len(rb.smallest)))
	fb = append(fb, rb.smallest...)
	fb = binary.AppendUvarint(fb, uint64(len(rb.largest)))
	fb = append(fb, rb.largest...)

	// Encode the top index and the magic.
	fb = binary.AppendUvarint(fb, uint64(len(rb.parts)))
	for _, p := range rb.parts {
		fb = binary.AppendUvarint(fb, uint64(len(p.key)))
		fb = append(fb, p.key...)
		fb = appendRef(fb, p.index)
		fb = appendRef(fb, p.filter)
	}
	fb = append(fb, footerMagic[:]...)

	// Place it and flush the last chunk.
	footerAt, err := rb.place(fb)
	if err != nil {
		return runMeta{}, err
	}
	if err := rb.flushChunk(); err != nil {
		return runMeta{}, err
	}
	return runMeta{
		chunks:   slices.Clone(rb.chunks[:rb.used]),
		count:    rb.count,
		smallest: rb.smallest,
		largest:  bytes.Clone(rb.largest),
		footer:   footerAt,
	}, nil
}

// unused returns the reserved chunks the run did not use.
func (rb *runBuilder) unused() []uint64 {
	return rb.chunks[rb.used:]
}

// appendRef encodes a location.
func appendRef(b []byte, r ref) []byte {
	b = binary.AppendUvarint(b, r.off)
	b = binary.AppendUvarint(b, uint64(r.n))
	return binary.LittleEndian.AppendUint32(b, r.crc)
}

// decodeRef reads a location.
func (d *decoder) decodeRef() ref {
	return ref{off: d.uvarint(), n: uint32(d.uvarint()), crc: d.u32()} // #nosec G115 -- appendRef wrote a uint32.
}

// runFooter is a run's decoded footer.
type runFooter struct {
	// count is the number of entries.
	count uint64
	// smallest and largest bound the keys.
	smallest, largest []byte
	// parts is the top index.
	parts []partition
}

// decodeRunFooter reads a footer.
func decodeRunFooter(b []byte) (*runFooter, error) {
	// Check the magic.
	if len(b) < 4 || [4]byte(b[len(b)-4:]) != footerMagic {
		return nil, errors.Wrap(ErrCorrupt, "run footer magic")
	}

	// Decode the fields.
	d := decoder{b: b[:len(b)-4]}
	f := &runFooter{count: d.uvarint(), smallest: d.bytes(), largest: d.bytes()}
	f.parts = make([]partition, d.count())
	for i := range f.parts {
		f.parts[i] = partition{key: d.bytes(), index: d.decodeRef(), filter: d.decodeRef()}
	}
	if d.err != nil {
		return nil, errors.Wrap(ErrCorrupt, "run footer truncated")
	}
	return f, nil
}

// decodeFences reads an index partition.
func decodeFences(b []byte) ([]fence, error) {
	// Read each fence's key and location.
	d := decoder{b: b}
	fs := make([]fence, d.count())
	for i := range fs {
		fs[i] = fence{key: d.bytes(), at: d.decodeRef()}
	}
	if d.err != nil {
		return nil, errors.Wrap(ErrCorrupt, "index partition truncated")
	}
	return fs, nil
}
