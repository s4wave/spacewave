package s4db

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/db/kvtx"
)

// build writes source into the empty file s with runs of runBytes each.
func build(ctx context.Context, s storage, opts Options, runBytes int64, source func(yield func(key, value []byte) error) error) error {
	// Lay out the header area and the empty log chunk, then stream.
	opts = opts.withDefaults()
	b := &builder{ctx: ctx, s: s, m: newManifest(), runBytes: runBytes}
	b.vals = valueChunkWriter{s: s, table: b.m.table}
	b.logChunk = b.m.table.alloc(1, chunkLog, 0)
	if err := source(b.add); err != nil {
		return err
	}
	return b.finish(opts)
}

// builder holds a Build in progress: the manifest it fills, the open run and
// value chunk, and the last key for the order check.
type builder struct {
	// ctx ends the build.
	ctx context.Context
	// s is the file.
	s storage
	// m is the manifest being filled.
	m *manifest
	// runBytes is the size a run closes at.
	runBytes int64
	// logChunk is the empty log chunk the superblock points at.
	logChunk uint64
	// vals writes the large values.
	vals valueChunkWriter
	// rb is the open run, nil between runs.
	rb *runBuilder
	// runs holds the finished runs in key order.
	runs []runMeta
	// runWritten counts the bytes of finished runs, which choose the level.
	runWritten int64
	// last is the last key added.
	last []byte
	// count is the number of pairs added.
	count int64
}

// add appends one pair.
func (b *builder) add(key, val []byte) error {
	// Check the pair and the order, and the context now and then.
	if err := b.check(key, val); err != nil {
		return err
	}
	b.last = append(b.last[:0], key...)
	b.count++

	// Place a large value in a value chunk.
	v := value{kind: valueInline, inline: val}
	if len(val) > inlineMax {
		ref, err := b.vals.put(key, val)
		if err != nil {
			return err
		}
		v = value{kind: valueRef, ref: ref}
	}

	// Add the entry to the open run, closing the run when it is full.
	if b.rb == nil {
		b.rb = newRunBuilder(b.s, nil, b.more)
	}
	if err := b.rb.add(key, v); err != nil {
		return err
	}
	if b.rb.written+int64(len(b.rb.buf)) >= b.runBytes {
		return b.closeRun()
	}
	return nil
}

// check refuses an invalid pair, one out of order, or an ended context.
func (b *builder) check(key, val []byte) error {
	switch {
	case len(key) == 0:
		return kvtx.ErrEmptyKey
	case len(key) > MaxKeySize:
		return ErrKeyTooLong
	case int64(len(val)) > MaxValueSize:
		return ErrValueTooLarge
	case b.count != 0 && bytes.Compare(key, b.last) <= 0:
		return ErrOutOfOrder
	case b.count%1024 == 0 && b.ctx.Err() != nil:
		return b.ctx.Err()
	}
	return nil
}

// more allocates the next run chunk.
func (b *builder) more() (uint64, error) {
	return b.m.table.alloc(1, chunkRun, 0), nil
}

// closeRun finishes the open run.
func (b *builder) closeRun() error {
	// Finish the run and account its bytes.
	meta, err := b.rb.finish()
	if err != nil {
		return err
	}
	b.runs = append(b.runs, meta)
	b.runWritten += b.rb.written
	b.rb = nil
	return nil
}

// finish closes the open run and value chunk, places the runs at their
// level, and writes the manifest, the superblock and the header.
func (b *builder) finish(opts Options) error {
	// Close what is open.
	if b.rb != nil && b.rb.count != 0 {
		if err := b.closeRun(); err != nil {
			return err
		}
	}
	if err := b.vals.flush(); err != nil {
		return err
	}

	// Place every run at the lowest level whose capacity holds them, and
	// name the runs by index. The runs of a record are seq*16+i, so the file
	// starts at the sequence that keeps the next record's ids above these.
	level, capacity := uint8(1), int64(runTarget*levelRatio)
	for capacity < b.runWritten {
		level++
		capacity *= levelRatio
	}
	for i := range b.runs {
		b.runs[i].id, b.runs[i].level = uint64(i), level // #nosec G115 -- an index.
		for _, c := range b.runs[i].chunks {
			b.m.table.at(c).owner = b.runs[i].id
		}
	}

	// Start the sequence where the next record's ids clear the runs'.
	var seq uint64
	if n := uint64(len(b.runs)); n > 16 {
		seq = (n+15)/16 - 1
	}
	b.m.runs, b.m.count = b.runs, b.count

	// Write the manifest into chunks at the end, taking one more until the
	// encoding, which grows with the table, fits.
	start := b.m.table.end()
	var mb []byte
	for n := uint64(1); ; n++ {
		*b.m.table.at(start + n - 1) = chunkEntry{kind: chunkManifest, owner: b.m.version}
		if mb = b.m.encode(); chunksFor(uint64(len(mb))) <= n {
			break
		}
	}
	if _, err := b.s.WriteAt(mb, fileOff(chunkStart(start))); err != nil {
		return err
	}
	if err := b.s.truncate(fileOff(chunkStart(b.m.table.end()))); err != nil {
		return err
	}
	if err := b.s.flushDurable(); err != nil {
		return err
	}

	// Point the first superblock at the manifest and the empty log, then
	// write the header last, so a crash before it leaves no database.
	sb := superblock{
		gen:      1,
		version:  b.m.version,
		manifest: ref{off: chunkStart(start), n: uint32(len(mb)), crc: checksum(mb)}, // #nosec G115 -- a manifest fits its chunks.
		seq:      seq,
		logPos:   chunkStart(b.logChunk),
		logEnd:   chunkStart(b.logChunk + 1),
		through:  seq,
		memPos:   chunkStart(b.logChunk),
		memEnd:   chunkStart(b.logChunk + 1),
	}
	if _, err := b.s.WriteAt(make([]byte, pageSize), slotPage*pageSize); err != nil {
		return err
	}
	if _, err := b.s.WriteAt(sb.encode(), sb.page()*pageSize); err != nil {
		return err
	}
	if _, err := b.s.WriteAt(newHeader(opts.memtable).encode(), headerPage*pageSize); err != nil {
		return err
	}
	return b.s.flushDurable()
}

// valueChunkWriter fills value chunks in stream order, one open chunk at a
// time, buffering the open chunk and writing it whole.
type valueChunkWriter struct {
	// s is the file.
	s storage
	// table accounts the chunks.
	table *chunkTable
	// chunk is the open chunk, zero before the first.
	chunk uint64
	// buf holds the open chunk's bytes.
	buf []byte
}

// put writes val under key and returns its reference: into its own chunks
// when the entry exceeds one, else into the open chunk, replaced when the
// entry does not fit.
func (w *valueChunkWriter) put(key, val []byte) (ref, error) {
	// An entry longer than a chunk takes a run of whole chunks.
	hdr := valueEntryHeaderLen(len(key), len(val))
	n := uint64(hdr + len(val)) // #nosec G115 -- lengths are not negative.
	if n > chunkSize {
		if err := w.flush(); err != nil {
			return ref{}, err
		}
		off := chunkStart(w.table.alloc(chunksFor(n), chunkValue, 0))
		entry := appendValueEntryHeader(make([]byte, 0, n), key, val)
		crc := binary.LittleEndian.Uint32(entry[len(entry)-4:])
		entry = append(entry, val...)
		if _, err := w.s.WriteAt(entry, fileOff(off)); err != nil {
			return ref{}, err
		}
		w.table.useValue(off, uint32(n), 0)                                    // #nosec G115 -- a value's length fits a reference.
		return ref{off: off + uint64(hdr), n: uint32(len(val)), crc: crc}, nil // #nosec G115 -- a value's length fits a reference.
	}

	// Open a chunk when none is open or the entry does not fit.
	if w.chunk == 0 || uint64(len(w.buf))+n > chunkSize {
		if err := w.flush(); err != nil {
			return ref{}, err
		}
		w.chunk = w.table.alloc(1, chunkValue, 0)
		w.buf = make([]byte, 0, chunkSize)
	}

	// Append the entry and account it.
	off := chunkStart(w.chunk) + uint64(len(w.buf))
	w.buf = appendValueEntryHeader(w.buf, key, val)
	crc := binary.LittleEndian.Uint32(w.buf[len(w.buf)-4:])
	w.buf = append(w.buf, val...)
	w.table.useValue(off, uint32(n), 0)                                    // #nosec G115 -- n fits a chunk.
	return ref{off: off + uint64(hdr), n: uint32(len(val)), crc: crc}, nil // #nosec G115 -- n fits a chunk.
}

// flush writes the open chunk's bytes.
func (w *valueChunkWriter) flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.s.WriteAt(w.buf, fileOff(chunkStart(w.chunk)))
	w.buf = w.buf[:0]
	return err
}
