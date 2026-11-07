package s4db

import (
	"bytes"
	"cmp"
	"sort"
	"sync"

	"github.com/pkg/errors"
)

// runReader reads one run: its footer stays in memory, and its partitions and
// blocks come from the block cache or the file.
type runReader struct {
	// db reads the file and holds the cache.
	db *DB
	// meta describes the run.
	meta runMeta
	// footer is the run's decoded footer.
	footer *runFooter
}

// runs caches the readers of the runs this handle has read, by run id. A
// run's footer is read once per handle; readers of runs a merge removed are
// dropped when their chunks are freed.
type runs struct {
	// mtx guards readers.
	mtx sync.Mutex
	// readers maps each run id to its reader.
	readers map[uint64]*runReader
}

// reader returns the reader of run m, reading its footer the first time.
func (db *DB) reader(m runMeta) (*runReader, error) {
	// Serve a known reader.
	db.runs.mtx.Lock()
	r := db.runs.readers[m.id]
	db.runs.mtx.Unlock()
	if r != nil {
		return r, nil
	}

	// Read and decode the footer.
	b, err := db.readChecked(m.footer, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "run %d footer", m.id)
	}
	f, err := decodeRunFooter(b)
	if err != nil {
		return nil, errors.Wrapf(err, "run %d footer", m.id)
	}

	// Keep the reader, or the one another goroutine kept meanwhile.
	r = &runReader{db: db, meta: m, footer: f}
	db.runs.mtx.Lock()
	if known := db.runs.readers[m.id]; known != nil {
		r = known
	} else {
		db.runs.readers[m.id] = r
	}
	db.runs.mtx.Unlock()
	return r, nil
}

// forgetRun drops the reader and cached blocks of a run a merge removed.
func (db *DB) forgetRun(id uint64) {
	db.runs.mtx.Lock()
	delete(db.runs.readers, id)
	db.runs.mtx.Unlock()
	db.cache.drop(id)
}

// readChecked reads the bytes at r into buf and checks them. With no buf the
// read gets a buffer of its exact size; a reused buf too small is replaced by
// one a quarter larger than r, so it settles at the largest block it holds.
func (db *DB) readChecked(r ref, buf []byte) ([]byte, error) {
	// Choose the buffer, then read and check.
	var b []byte
	switch n := int(r.n); {
	case buf == nil:
		b = make([]byte, n)
	case cap(buf) < n:
		b = make([]byte, n, n+n/4)
	default:
		b = buf[:n]
	}
	if _, err := db.s.ReadAt(b, fileOff(r.off)); err != nil {
		return nil, err
	}
	if checksum(b) != r.crc {
		return nil, errors.Wrapf(ErrCorrupt, "checksum mismatch at %d", r.off)
	}
	return b, nil
}

// block returns the bytes at r through the cache. A nil scratch keeps a read
// the cache missed. Otherwise the read lands in *scratch and the cache is left
// alone, for a background pass; those bytes last until the caller's next read
// into the same scratch.
func (rr *runReader) block(r ref, scratch *[]byte) ([]byte, error) {
	// Serve the cache.
	k := cacheKey{run: rr.meta.id, off: r.off}
	if b := rr.db.cache.get(k); b != nil {
		return b, nil
	}

	// Read, then keep the block or the buffer.
	var buf []byte
	if scratch != nil {
		buf = *scratch
	}
	b, err := rr.db.readChecked(r, buf)
	if err != nil {
		return nil, err
	}
	if scratch == nil {
		rr.db.cache.put(k, b)
	} else {
		*scratch = b
	}
	return b, nil
}

// partitionFor returns the index of the partition whose blocks may hold key,
// or -1 when key is below the run.
func (rr *runReader) partitionFor(key []byte) int {
	parts := rr.footer.parts
	i := sort.Search(len(parts), func(i int) bool { return bytes.Compare(parts[i].key, key) > 0 })
	return i - 1
}

// index returns index partition i, read in place, through scratch as block
// describes.
func (rr *runReader) index(i int, scratch *[]byte) (fenceIndex, error) {
	b, err := rr.block(rr.footer.parts[i].index, scratch)
	if err != nil {
		return fenceIndex{}, err
	}
	return decodeFenceIndex(b)
}

// getScratch holds the reads of a lookup that leaves the block cache alone,
// as a background pass does. An inline value found through it lasts until the
// next lookup through the same scratch.
type getScratch struct {
	// filter, index and block hold the partitions and block read.
	filter, index, block []byte
}

// bufs returns the buffers the filter, index and block reads land in, all
// nil to read through the cache when sc is nil.
func (sc *getScratch) bufs() (filter, index, block *[]byte) {
	if sc == nil {
		return nil, nil, nil
	}
	return &sc.filter, &sc.index, &sc.block
}

// get returns the value of key in the run, reading through the cache, or
// through sc around it when sc is set.
func (rr *runReader) get(key []byte, sc *getScratch) (value, bool, error) {
	// Find the partition, and ask its filter.
	if bytes.Compare(key, rr.footer.smallest) < 0 || bytes.Compare(key, rr.footer.largest) > 0 {
		return value{}, false, nil
	}
	i := rr.partitionFor(key)
	if i < 0 {
		return value{}, false, nil
	}
	fbuf, xbuf, bbuf := sc.bufs()
	fb, err := rr.block(rr.footer.parts[i].filter, fbuf)
	if err != nil {
		return value{}, false, err
	}
	if !decodeRunFilter(fb).has(key) {
		return value{}, false, nil
	}

	// Find the block by its fence.
	x, err := rr.index(i, xbuf)
	if err != nil {
		return value{}, false, err
	}
	j, err := x.floor(key)
	if err != nil || j < 0 {
		return value{}, false, err
	}
	f, err := x.at(j)
	if err != nil {
		return value{}, false, err
	}

	// Search the block.
	bb, err := rr.block(f.at, bbuf)
	if err != nil {
		return value{}, false, err
	}
	blk, err := decodeBlock(bb)
	if err != nil {
		return value{}, false, err
	}
	v, ok := blk.find(key)
	return v, ok, nil
}

// lookupRuns returns the value of key in the newest run of st that holds it.
// A tombstone found first hides the key. Level 0's runs overlap and are read
// newest first; each lower level's runs do not, so only the one whose range
// covers the key is read, found by bisection. A lookup reads R0 + L runs at
// most, and tests each one's range before it touches the run's reader. A
// lookup through sc leaves the block cache alone.
func (db *DB) lookupRuns(st *state, key []byte, sc *getScratch) (value, bool, error) {
	runs := st.m.runs
	for len(runs) != 0 {
		// Split off the next level, which lookupOrder keeps contiguous.
		level := runs[0].level
		n := sort.Search(len(runs), func(i int) bool { return runs[i].level != level })
		lv := runs[:n]
		runs = runs[n:]

		// A lower level holds key in at most its first run ending at or
		// after it.
		if level != 0 {
			i := sort.Search(len(lv), func(i int) bool { return bytes.Compare(lv[i].largest, key) >= 0 })
			lv = lv[i:min(i+1, len(lv))]
		}

		// Read each candidate run whose range holds key.
		for i := range lv {
			m := &lv[i]
			if bytes.Compare(key, m.smallest) < 0 || bytes.Compare(key, m.largest) > 0 {
				continue
			}
			rr, err := db.reader(*m)
			if err != nil {
				return value{}, false, err
			}
			v, ok, err := rr.get(key, sc)
			if err != nil {
				return value{}, false, err
			}
			if ok {
				return v, v.kind != valueDeleted, nil
			}
		}
	}
	return value{}, false, nil
}

// level0 counts the level 0 runs.
func (m *manifest) level0() int {
	return len(m.level(0))
}

// lookupOrder orders runs as a lookup reads them: level 0 newest first, then
// each lower level in key order, whose runs do not overlap.
func lookupOrder(a, b runMeta) int {
	if a.level != b.level {
		return int(a.level) - int(b.level)
	}
	if a.level == 0 {
		return cmp.Compare(b.id, a.id)
	}
	return bytes.Compare(a.smallest, b.smallest)
}

// runLayers returns the merge layers of st's runs that may hold keys under
// prefix, newest first: one per level 0 run, whose runs overlap, and one per
// lower level, whose runs are in key order and do not, over the runs from the
// first ending at or after the prefix, found by bisection, to the last
// starting before its end. A scan's layers grow with level 0 and the levels,
// not with the runs it crosses.
func (db *DB) runLayers(st *state, prefix []byte) []layer {
	// Walk the levels in lookup order.
	var layers []layer
	end := prefixEnd(prefix)
	runs := st.m.runs
	for len(runs) != 0 {
		// Split off the next level, which lookupOrder keeps contiguous.
		level := runs[0].level
		n := sort.Search(len(runs), func(i int) bool { return runs[i].level != level })
		lv := runs[:n]
		runs = runs[n:]

		// Level 0 gives a layer per run whose range meets the prefix.
		if level == 0 {
			for _, m := range lv {
				if bytes.Compare(m.largest, prefix) >= 0 && (end == nil || bytes.Compare(m.smallest, end) < 0) {
					layers = append(layers, &runIter{db: db, meta: m, cache: true})
				}
			}
			continue
		}

		// A lower level gives one layer over its runs in range.
		lo := sort.Search(len(lv), func(i int) bool { return bytes.Compare(lv[i].largest, prefix) >= 0 })
		hi := len(lv)
		if end != nil {
			hi = sort.Search(len(lv), func(i int) bool { return bytes.Compare(lv[i].smallest, end) >= 0 })
		}
		if lo < hi {
			layers = append(layers, newLevelIter(db, lv[lo:hi]))
		}
	}
	return layers
}

// runIter iterates one run as a merge layer, one block at a time. It steps in
// the direction of its last seek. Forward, it decodes a block's entries as it
// steps, from the restart point its seek starts at, so a short scan decodes a
// few entries rather than every block it touches; in reverse it decodes each
// block whole, since stepping back needs the entries before.
type runIter struct {
	// db reads the run.
	db *DB
	// meta describes the run.
	meta runMeta
	// cache reads through the block cache. Scans do; background jobs do not,
	// so a merge never displaces the blocks foreground reads use, and read
	// into indexBuf and blockBuf instead, which each load reuses. An entry
	// of such a layer lasts only until its next step.
	cache bool
	// indexBuf and blockBuf hold the index partition and block read around
	// the cache.
	indexBuf, blockBuf []byte
	// rr is the run's reader, set on first use.
	rr *runReader
	// part is the current partition and index its index partition, empty
	// before the first.
	part  int
	index fenceIndex
	// blk is the current block's index among the fences and data the block.
	blk  int
	data dataBlock
	// entries holds the block's decoded entries from where decoding started,
	// keys their keys, reused by the next block, and more the offset of the
	// first entry not yet decoded.
	entries []blockEntry
	keys    []byte
	more    int
	// i indexes the current entry in entries; -1 before the first.
	i int
	// ok reports whether the layer is on an entry.
	ok bool
	// err is the first error.
	err error
}

// open loads the reader.
func (it *runIter) open() bool {
	if it.rr != nil || it.err != nil {
		return it.err == nil
	}
	it.rr, it.err = it.db.reader(it.meta)
	return it.err == nil
}

// scratch returns buf for a read around the cache, or nil to read through it.
func (it *runIter) scratch(buf *[]byte) *[]byte {
	if it.cache {
		return nil
	}
	return buf
}

// loadIndex loads the index partition of partition p.
func (it *runIter) loadIndex(p int) bool {
	// Keep the loaded index, else read the partition's.
	if it.index.len() != 0 && it.part == p {
		return true
	}
	x, err := it.rr.index(p, it.scratch(&it.indexBuf))
	if err != nil {
		it.err = err
		return false
	}
	it.part, it.index = p, x
	return true
}

// loadBlock reads block j of partition p and decodes its first entry, or
// every entry in reverse, reporting whether it holds one.
func (it *runIter) loadBlock(p, j int, reverse bool) bool {
	// Find the block's fence in the partition's index.
	if !it.loadIndex(p) || j < 0 || j >= it.index.len() {
		return false
	}
	f, err := it.index.at(j)
	if err != nil {
		it.err = err
		return false
	}

	// Read the block.
	b, err := it.rr.block(f.at, it.scratch(&it.blockBuf))
	if err != nil {
		it.err = err
		return false
	}
	blk, err := decodeBlock(b)
	if err != nil {
		it.err = err
		return false
	}

	// Decode from its start.
	it.blk, it.data = j, blk
	it.entries, it.keys, it.more = it.entries[:0], it.keys[:0], 0
	if !it.decode() {
		return false
	}
	for reverse && it.decode() {
	}
	return it.err == nil
}

// decode appends the entry at more to entries, reporting whether the block
// held one. more is the block's start, a restart point, or the entry after
// the last decoded one.
func (it *runIter) decode() bool {
	// Stop at the block's end; an entry after the first shares its prefix
	// with the one before.
	if it.more >= len(it.data.data) {
		return false
	}
	var prev []byte
	if n := len(it.entries); n != 0 {
		prev = it.entries[n-1].key
	}
	e, keys, ok := it.data.entryAt(it.more, prev, it.keys)
	if !ok {
		it.err = errors.Wrap(ErrCorrupt, "block entry")
		return false
	}
	it.keys, it.more = keys, e.next
	it.entries = append(it.entries, e)
	return true
}

// seekBlock positions a forward layer at the first key at or after k in the
// current block, decoding from the last restart point at or before k, or at
// the next block's first key when this block holds none.
func (it *runIter) seekBlock(k []byte) {
	// Restart decoding at the restart point.
	off, keys, ok := it.data.restart(k, it.keys[:0])
	if !ok {
		it.err = errors.Wrap(ErrCorrupt, "block restart")
		return
	}
	it.entries, it.keys, it.more = it.entries[:0], keys[:0], off

	// Decode up to the first key at or after k.
	for it.decode() {
		if bytes.Compare(it.entries[len(it.entries)-1].key, k) >= 0 {
			it.i, it.ok = len(it.entries)-1, true
			return
		}
	}
	if it.err != nil || len(it.entries) == 0 {
		return
	}
	it.i, it.ok = len(it.entries)-1, true
	it.step(false)
}

// failure returns the first read or decode error.
func (it *runIter) failure() error {
	return it.err
}

// valid reports whether the layer is on an entry.
func (it *runIter) valid() bool {
	return it.ok && it.err == nil
}

// key returns the current key.
func (it *runIter) key() []byte {
	return it.entries[it.i].key
}

// entry returns the current entry.
func (it *runIter) entry() ientry {
	v := it.entries[it.i].val
	return ientry{del: v.kind == valueDeleted, val: v}
}

// seek positions at the first key at or after k, or the last at or before it
// when reverse; nil selects the first or last key.
func (it *runIter) seek(k []byte, reverse bool) {
	// Find the partition and block for k, or the run's ends.
	it.ok = false
	if !it.open() {
		return
	}
	f := it.rr.footer
	if len(f.parts) == 0 {
		return
	}
	p, j := 0, 0
	switch {
	case k == nil && reverse:
		p = len(f.parts) - 1
		if !it.loadIndex(p) {
			return
		}
		j = it.index.len() - 1
	case k == nil:
	default:
		p = max(it.rr.partitionFor(k), 0)
		if !it.loadIndex(p) {
			return
		}
		fj, err := it.index.floor(k)
		if err != nil {
			it.err = err
			return
		}
		j = max(fj, 0)
	}
	if !it.loadBlock(p, j, reverse) {
		return
	}

	// Position within the block, then settle across block edges.
	if k == nil {
		it.i = 0
		if reverse {
			it.i = len(it.entries) - 1
		}
		it.ok = true
		return
	}
	if !reverse {
		it.seekBlock(k)
		return
	}
	it.i = sort.Search(len(it.entries), func(i int) bool { return bytes.Compare(it.entries[i].key, k) >= 0 })
	if it.i < len(it.entries) && bytes.Equal(it.entries[it.i].key, k) {
		it.ok = true
		return
	}
	it.i--
	it.ok = true
	if it.i < 0 {
		it.i = 0
		it.step(true)
	}
}

// step moves to the next key in the direction, crossing blocks and
// partitions.
func (it *runIter) step(reverse bool) {
	// Back up through decoded entries, then whole blocks before.
	if !it.ok {
		return
	}
	if reverse {
		it.i--
		for it.i < 0 {
			if !it.prevBlock() {
				it.ok = false
				return
			}
			it.i = len(it.entries) - 1
		}
		return
	}

	// Step forward, decoding the next entry, then into the next block.
	it.i++
	if it.i < len(it.entries) || it.decode() {
		return
	}
	if it.err != nil || !it.nextBlock() {
		it.ok = false
		return
	}
	it.i = 0
}

// nextBlock loads the block after the current one.
func (it *runIter) nextBlock() bool {
	if it.blk+1 < it.index.len() {
		return it.loadBlock(it.part, it.blk+1, false)
	}
	if it.part+1 < len(it.rr.footer.parts) {
		return it.loadBlock(it.part+1, 0, false)
	}
	return false
}

// prevBlock loads the block before the current one.
func (it *runIter) prevBlock() bool {
	if it.blk > 0 {
		return it.loadBlock(it.part, it.blk-1, true)
	}
	if it.part > 0 {
		if !it.loadIndex(it.part - 1) {
			return false
		}
		return it.loadBlock(it.part, it.index.len()-1, true)
	}
	return false
}

// _ is a type assertion
var _ layer = (*runIter)(nil)
