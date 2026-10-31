package s4db

import (
	"bytes"
	"slices"
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
	b, err := db.readChecked(m.footer)
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

// readChecked reads the bytes at r and checks them.
func (db *DB) readChecked(r ref) ([]byte, error) {
	b := make([]byte, r.n)
	if _, err := db.s.ReadAt(b, fileOff(r.off)); err != nil {
		return nil, err
	}
	if checksum(b) != r.crc {
		return nil, errors.Wrapf(ErrCorrupt, "checksum mismatch at %d", r.off)
	}
	return b, nil
}

// block returns the bytes at r through the cache; cache false reads around
// it, for a background pass.
func (rr *runReader) block(r ref, cache bool) ([]byte, error) {
	// Serve the cache, else read and maybe keep.
	k := cacheKey{run: rr.meta.id, off: r.off}
	if b := rr.db.cache.get(k); b != nil {
		return b, nil
	}
	b, err := rr.db.readChecked(r)
	if err != nil {
		return nil, err
	}
	if cache {
		rr.db.cache.put(k, b)
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

// fences returns the decoded index partition i.
func (rr *runReader) fences(i int, cache bool) ([]fence, error) {
	b, err := rr.block(rr.footer.parts[i].index, cache)
	if err != nil {
		return nil, err
	}
	return decodeFences(b)
}

// get returns the value of key in the run.
func (rr *runReader) get(key []byte) (value, bool, error) {
	// Find the partition, and ask its filter.
	if bytes.Compare(key, rr.footer.smallest) < 0 || bytes.Compare(key, rr.footer.largest) > 0 {
		return value{}, false, nil
	}
	i := rr.partitionFor(key)
	if i < 0 {
		return value{}, false, nil
	}
	fb, err := rr.block(rr.footer.parts[i].filter, true)
	if err != nil {
		return value{}, false, err
	}
	if !decodeRunFilter(fb).has(key) {
		return value{}, false, nil
	}

	// Find the block by its fence.
	fences, err := rr.fences(i, true)
	if err != nil {
		return value{}, false, err
	}
	j := sort.Search(len(fences), func(j int) bool { return bytes.Compare(fences[j].key, key) > 0 }) - 1
	if j < 0 {
		return value{}, false, nil
	}

	// Search the block.
	bb, err := rr.block(fences[j].at, true)
	if err != nil {
		return value{}, false, err
	}
	blk, err := decodeBlock(bb)
	if err != nil {
		return value{}, false, err
	}
	e, ok := blk.seek(key)
	if !ok || !bytes.Equal(e.key, key) {
		return value{}, false, nil
	}
	return e.val, true, nil
}

// lookupRuns returns the value of key in the newest run of st that holds it.
// A tombstone found first hides the key.
func (db *DB) lookupRuns(st *state, key []byte) (value, bool, error) {
	for _, m := range st.m.runsNewestFirst() {
		rr, err := db.reader(m)
		if err != nil {
			return value{}, false, err
		}
		v, ok, err := rr.get(key)
		if err != nil {
			return value{}, false, err
		}
		if ok {
			return v, v.kind != valueDeleted, nil
		}
	}
	return value{}, false, nil
}

// level0 counts the level 0 runs.
func (m *manifest) level0() int {
	n := 0
	for _, r := range m.runs {
		if r.level == 0 {
			n++
		}
	}
	return n
}

// runsNewestFirst returns the runs in lookup order: level 0 newest first,
// then each lower level, whose runs do not overlap.
func (m *manifest) runsNewestFirst() []runMeta {
	rs := slices.Clone(m.runs)
	slices.SortStableFunc(rs, func(a, b runMeta) int {
		if a.level != b.level {
			return int(a.level) - int(b.level)
		}
		if a.level == 0 {
			if a.id > b.id {
				return -1
			}
			if a.id < b.id {
				return 1
			}
			return 0
		}
		return bytes.Compare(a.smallest, b.smallest)
	})
	return rs
}

// runLayers returns one merge layer per run of st that may hold keys under
// prefix, newest first.
func (db *DB) runLayers(st *state, prefix []byte) []layer {
	var layers []layer
	end := prefixEnd(prefix)
	for _, m := range st.m.runsNewestFirst() {
		if bytes.Compare(m.largest, prefix) < 0 || (end != nil && bytes.Compare(m.smallest, end) >= 0) {
			continue
		}
		layers = append(layers, &runIter{db: db, meta: m})
	}
	return layers
}

// runIter iterates one run as a merge layer, one decoded block at a time.
type runIter struct {
	// db reads the run.
	db *DB
	// meta describes the run.
	meta runMeta
	// rr is the run's reader, set on first use.
	rr *runReader
	// part is the current partition and fences its fences.
	part   int
	fences []fence
	// blk is the current block's index among fences, and entries its decoded
	// entries.
	blk     int
	entries []blockEntry
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

// loadBlock decodes block j of partition p into entries.
func (it *runIter) loadBlock(p, j int) bool {
	// Load the partition's fences when the partition changed.
	if it.fences == nil || it.part != p {
		fs, err := it.rr.fences(p, false)
		if err != nil {
			it.err = err
			return false
		}
		it.part, it.fences = p, fs
	}
	if j < 0 || j >= len(it.fences) {
		return false
	}

	// Read the block.
	b, err := it.rr.block(it.fences[j].at, false)
	if err != nil {
		it.err = err
		return false
	}
	blk, err := decodeBlock(b)
	if err != nil {
		it.err = err
		return false
	}

	// Decode every entry.
	it.entries = it.entries[:0]
	var prev []byte
	for off := 0; off < len(blk.data); {
		e, ok := blk.entryAt(off, prev)
		if !ok {
			it.err = errors.Wrap(ErrCorrupt, "block entry")
			return false
		}
		it.entries = append(it.entries, e)
		prev, off = e.key, e.next
	}
	it.blk = j
	return len(it.entries) != 0
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
		if !it.loadBlock(p, 0) {
			return
		}
		j = len(it.fences) - 1
	case k == nil:
	default:
		p = max(it.rr.partitionFor(k), 0)
		if !it.loadBlock(p, 0) {
			return
		}
		j = max(sort.Search(len(it.fences), func(j int) bool { return bytes.Compare(it.fences[j].key, k) > 0 })-1, 0)
	}
	if !it.loadBlock(p, j) {
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
	it.i = sort.Search(len(it.entries), func(i int) bool { return bytes.Compare(it.entries[i].key, k) >= 0 })
	if reverse {
		if it.i < len(it.entries) && bytes.Equal(it.entries[it.i].key, k) {
			it.ok = true
			return
		}
		it.i--
		if it.i < 0 {
			it.ok = true
			it.i = 0
			it.step(true)
			return
		}
		it.ok = true
		return
	}
	if it.i >= len(it.entries) {
		it.ok = true
		it.i = len(it.entries) - 1
		it.step(false)
		return
	}
	it.ok = true
}

// step moves to the next key in the direction, crossing blocks and
// partitions.
func (it *runIter) step(reverse bool) {
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
	it.i++
	for it.i >= len(it.entries) {
		if !it.nextBlock() {
			it.ok = false
			return
		}
		it.i = 0
	}
}

// nextBlock loads the block after the current one.
func (it *runIter) nextBlock() bool {
	if it.blk+1 < len(it.fences) {
		return it.loadBlock(it.part, it.blk+1)
	}
	if it.part+1 < len(it.rr.footer.parts) {
		return it.loadBlock(it.part+1, 0)
	}
	return false
}

// prevBlock loads the block before the current one.
func (it *runIter) prevBlock() bool {
	if it.blk > 0 {
		return it.loadBlock(it.part, it.blk-1)
	}
	if it.part > 0 {
		if !it.loadBlock(it.part-1, 0) {
			return false
		}
		return it.loadBlock(it.part-1, len(it.fences)-1)
	}
	return false
}

// _ is a type assertion
var _ layer = (*runIter)(nil)
