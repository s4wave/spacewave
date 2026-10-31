package s4db

import (
	"encoding/binary"
)

// Record kinds.
const (
	// kindCommit is a committed write transaction, or the cleaner's moves.
	kindCommit = 1
	// kindLink continues the log in another chunk.
	kindLink = 2
	// kindReserve marks chunks a background job will write, owned by the
	// slot of its process.
	kindReserve = 3
	// kindFlush adds a level 0 run holding the memtable through a commit.
	kindFlush = 4
	// kindMerge replaces runs with the runs merged from them.
	kindMerge = 5
	// kindFree returns chunks to free, pending the pins.
	kindFree = 6
	// kindSealed marks a value chunk the cleaner emptied, pending the pins.
	kindSealed = 7
	// kindTruncate ends the table at a chunk; the file is truncated to it.
	kindTruncate = 8
)

// recordHeader is the length, checksum, sequence, kind, writer identity,
// predecessor identity and durable mark of a record. The checksum covers
// everything after itself and continues from the previous record's. A reader
// accepts a record only when its predecessor identity is the previous
// record's writer, so a writer that continues a log cut by recovery never
// joins the stale records behind the cut.
const recordHeader = 4 + 4 + 8 + 1 + 8 + 8 + 8

// linkSize is the length of a link record, which every log chunk keeps room
// for at its end.
const linkSize = recordHeader + 2*10

// record is one decoded log record.
type record struct {
	// seq is the record sequence.
	seq uint64
	// kind is the record kind.
	kind byte
	// writer is the identity of the handle that wrote the record.
	writer uint64
	// prevWriter is the writer of the record before it.
	prevWriter uint64
	// durable is the last commit the writer had flushed when it wrote this
	// record. Recovery checks the values of records past the highest mark.
	durable uint64

	// delta is a commit's change in key count.
	delta int64
	// hot and cold are the open value chunks after a commit, 0 for none.
	hot, cold uint64
	// ops are a commit's key changes in key order.
	ops []op
	// frees are the value chunk entries a commit freed, headers included.
	frees []ref
	// next is the first chunk a link record continues in, and span how many
	// consecutive chunks the log takes there.
	next, span uint64
	// chunks are the chunks a reserve, free or sealed record names, or a
	// flush or merge record's output chunks follow their runs.
	chunks []uint64
	// slot is the reserving process's slot in a reserve record.
	slot uint64
	// runs are the runs a flush or merge record adds, and mchunks the chunks
	// holding the manifest written with it.
	runs    []runMeta
	mchunks []uint64
	// removed are the run ids a merge record removes.
	removed []uint64
	// through is the last commit a flush record's run holds, and replay
	// locates the first record after it, where the superblock the flush
	// publishes rebuilds the memtable from.
	through uint64
	replay  logPoint

	// size is the encoded length.
	size int
	// crc is the record's checksum.
	crc uint32
}

// op is one key change in a commit record.
type op struct {
	// key is the key.
	key []byte
	// val is the value, a tombstone, or a move.
	val value
}

// decodeRecord reads the record at the start of b, reporting false when b
// holds no complete valid record with sequence seq following the record with
// checksum prev written by prevWriter.
func decodeRecord(b []byte, seq uint64, prev uint32, prevWriter uint64) (*record, bool) {
	// Check that a whole record with the expected sequence and predecessor
	// is present.
	if len(b) < recordHeader {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < recordHeader || n > len(b) || binary.LittleEndian.Uint64(b[8:]) != seq {
		return nil, false
	}
	if binary.LittleEndian.Uint64(b[25:]) != prevWriter {
		return nil, false
	}
	crc := chain(prev, b[8:n])
	if crc != binary.LittleEndian.Uint32(b[4:]) {
		return nil, false
	}

	// Decode the body by kind.
	r := &record{seq: seq, kind: b[16], writer: binary.LittleEndian.Uint64(b[17:]), prevWriter: prevWriter, durable: binary.LittleEndian.Uint64(b[33:]), size: n, crc: crc}
	d := decoder{b: b[recordHeader:n]}
	switch r.kind {
	case kindCommit:
		r.decodeCommit(&d)
	case kindLink:
		r.next = d.uvarint()
		r.span = d.uvarint()
	case kindReserve:
		r.slot = d.uvarint()
		r.chunks = d.chunkList()
	case kindFlush:
		r.through = d.uvarint()
		r.replay = logPoint{pos: d.uvarint(), end: d.uvarint(), crc: d.u32(), writer: d.uvarint()}
		r.runs = d.runList()
		r.mchunks = d.chunkList()
	case kindMerge:
		r.removed = d.chunkList()
		r.runs = d.runList()
		r.mchunks = d.chunkList()
	case kindFree, kindSealed:
		r.chunks = d.chunkList()
	case kindTruncate:
		r.next = d.uvarint()
	default:
		return nil, false
	}
	return r, d.err == nil
}

// decodeCommit reads a commit record's body.
func (r *record) decodeCommit(d *decoder) {
	// Read the bookkeeping.
	r.delta = d.varint()
	r.hot = d.uvarint()
	r.cold = d.uvarint()

	// Read the key changes.
	r.ops = make([]op, d.count())
	for i := range r.ops {
		r.ops[i].key = d.bytes()
		r.ops[i].val = d.decodeValue(d.kind())
	}

	// Read the freed value bytes.
	r.frees = make([]ref, d.count())
	for i := range r.frees {
		r.frees[i] = ref{off: d.uvarint(), n: uint32(d.uvarint())} // #nosec G115 -- encode wrote a uint32.
	}
}

// chunkList reads a counted list of chunk numbers.
func (d *decoder) chunkList() []uint64 {
	cs := make([]uint64, d.count())
	for i := range cs {
		cs[i] = d.uvarint()
	}
	return cs
}

// runList reads a counted list of run descriptions.
func (d *decoder) runList() []runMeta {
	rs := make([]runMeta, d.count())
	for i := range rs {
		rs[i].decode(d)
	}
	return rs
}

// encode returns the encoding of r following the record with checksum prev,
// and sets r.crc and r.size.
func (r *record) encode(prev uint32) []byte {
	b := make([]byte, recordHeader, recordHeader+r.bound())
	switch r.kind {
	case kindCommit:
		b = r.appendCommit(b)
	case kindLink:
		b = binary.AppendUvarint(b, r.next)
		b = binary.AppendUvarint(b, r.span)
	case kindReserve:
		b = binary.AppendUvarint(b, r.slot)
		b = appendChunkList(b, r.chunks)
	case kindFlush:
		b = binary.AppendUvarint(b, r.through)
		b = binary.AppendUvarint(b, r.replay.pos)
		b = binary.AppendUvarint(b, r.replay.end)
		b = binary.LittleEndian.AppendUint32(b, r.replay.crc)
		b = binary.AppendUvarint(b, r.replay.writer)
		b = appendRunList(b, r.runs)
		b = appendChunkList(b, r.mchunks)
	case kindMerge:
		b = appendChunkList(b, r.removed)
		b = appendRunList(b, r.runs)
		b = appendChunkList(b, r.mchunks)
	case kindFree, kindSealed:
		b = appendChunkList(b, r.chunks)
	case kindTruncate:
		b = binary.AppendUvarint(b, r.next)
	}
	return r.seal(b, prev)
}

// appendCommit appends a commit record's body.
func (r *record) appendCommit(b []byte) []byte {
	// Write the bookkeeping.
	b = binary.AppendVarint(b, r.delta)
	b = binary.AppendUvarint(b, r.hot)
	b = binary.AppendUvarint(b, r.cold)

	// Write the key changes.
	b = binary.AppendUvarint(b, uint64(len(r.ops)))
	for _, o := range r.ops {
		b = binary.AppendUvarint(b, uint64(len(o.key)))
		b = append(b, o.key...)
		b = o.val.append(b)
	}

	// Write the freed value bytes.
	b = binary.AppendUvarint(b, uint64(len(r.frees)))
	for _, f := range r.frees {
		b = binary.AppendUvarint(b, f.off)
		b = binary.AppendUvarint(b, uint64(f.n))
	}
	return b
}

// appendChunkList appends a counted list of chunk numbers.
func appendChunkList(b []byte, cs []uint64) []byte {
	b = binary.AppendUvarint(b, uint64(len(cs)))
	for _, c := range cs {
		b = binary.AppendUvarint(b, c)
	}
	return b
}

// appendRunList appends a counted list of run descriptions.
func appendRunList(b []byte, rs []runMeta) []byte {
	b = binary.AppendUvarint(b, uint64(len(rs)))
	for i := range rs {
		b = rs[i].append(b)
	}
	return b
}

// bound returns an upper bound on the encoded length of r's body.
func (r *record) bound() int {
	// Count the fixed fields, the ops, the frees, the chunk lists and the
	// runs.
	n := 8 * binary.MaxVarintLen64
	for _, o := range r.ops {
		n += binary.MaxVarintLen64 + len(o.key) + o.val.size()
	}
	n += 2 * binary.MaxVarintLen64 * len(r.frees)
	n += binary.MaxVarintLen64 * (len(r.chunks) + len(r.removed) + len(r.mchunks) + 3)
	for i := range r.runs {
		n += r.runs[i].bound()
	}
	return n
}

// seal fills in the record header of b following the record with checksum
// prev.
func (r *record) seal(b []byte, prev uint32) []byte {
	// Write the length, sequence, kind, identities and mark.
	binary.LittleEndian.PutUint32(b[0:], uint32(len(b))) // #nosec G115 -- a record is far below 4 GiB.
	binary.LittleEndian.PutUint64(b[8:], r.seq)
	b[16] = r.kind
	binary.LittleEndian.PutUint64(b[17:], r.writer)
	binary.LittleEndian.PutUint64(b[25:], r.prevWriter)
	binary.LittleEndian.PutUint64(b[33:], r.durable)

	// Seal with the checksum over everything after it.
	r.crc = chain(prev, b[8:])
	binary.LittleEndian.PutUint32(b[4:], r.crc)
	r.size = len(b)
	return b
}

// own copies the keys and inline values of a decoded record out of the log
// reader's window into one allocation, so the memtable entries a kept record
// becomes do not pin every window replay read.
func (r *record) own() {
	// Size the copy.
	n := 0
	for _, o := range r.ops {
		n += len(o.key) + len(o.val.inline)
	}

	// Copy each key and inline value, keeping a nil value nil.
	b := make([]byte, 0, n)
	for i := range r.ops {
		o := &r.ops[i]
		at := len(b)
		b = append(b, o.key...)
		o.key = b[at:len(b):len(b)]
		if o.val.inline != nil {
			at = len(b)
			b = append(b, o.val.inline...)
			o.val.inline = b[at:len(b):len(b)]
		}
	}
}
