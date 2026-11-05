package s4db

import (
	"bytes"
	"encoding/binary"
	"slices"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
)

// A delta is a tree a checkpoint writes from the overlay alone, without
// reading or rewriting the base tree: the entries committed since the
// checkpoint before, deletes kept as tombstones. The superblock names the
// deltas newest first, and a lookup reads the overlay, then each delta whose
// filter may hold the key, then the base. A merge folds the oldest deltas, its
// round, into the base one bounded step of keys at a time, and drops them
// once every key of the round is in the base. Until then the round stays
// named above the base, so a key the base already holds from the round reads
// the same value from the delta.
//
// Invariants:
//   - A lookup through a state's overlay, deltas and base returns the value
//     of the state's last commit to each key (Runs.tla ReadsLive).
//   - The superblock a crash recovers names only pages written and flushed
//     before it, so recovery reads every committed key (RecoversCommitted).
//   - Pages a merge step or a dropped delta frees wait for a checkpoint after
//     the one the step saved under, and for every snapshot to pass it, so no
//     reader or recoverable superblock names a free page (FreeUnnamed).

// maxDeltas bounds the deltas a superblock names. A checkpoint at the bound
// merges instead of writing a delta.
const maxDeltas = 64

// mergeDeltas is the delta count at which a merge starts.
const mergeDeltas = 16

// mergeStepKeys bounds the keys one merge step folds into the base. Each
// rewrites up to a leaf, so a step writes at most about this many pages.
// Tests lower it to merge in many steps.
var mergeStepKeys = 16384

// mergeFreedMax bounds, in pages, what merge steps free before the next
// checkpoint. The pages wait for that checkpoint, so past the bound the
// writer checkpoints before it merges more.
const mergeFreedMax = 256 << 20 / pageSize

// deltaFilterBits is the filter size per key.
const deltaFilterBits = 10

// deltaRef names a delta in the superblock.
type deltaRef struct {
	// root is the root page of the delta's tree.
	root uint64
	// head is the page holding the filter's location.
	head uint64
	// pages is the number of pages the delta occupies: its tree, its head
	// and its filter.
	pages uint64
}

// delta is a loaded delta. It is immutable and shared by every state that
// names it.
type delta struct {
	deltaRef
	// filter holds the words of a blocked Bloom filter over the delta's keys.
	filter []uint64
	// filterPages lists the pages holding the filter.
	filterPages []run
}

// has reports whether the delta may hold key.
func (d *delta) has(key []byte) bool {
	i, mask := blockProbe(xxhash.Sum64(key), len(d.filter))
	return d.filter[i]&mask == mask
}

// newDeltaFilter returns the words of a filter sized for n keys.
func newDeltaFilter(n int) []uint64 {
	return make([]uint64, max(1, n*deltaFilterBits/64))
}

// addDeltaFilter adds key to the filter words.
func addDeltaFilter(words []uint64, key []byte) {
	i, mask := blockProbe(xxhash.Sum64(key), len(words))
	words[i] |= mask
}

// Delta head page layout: the filter's word count, its page run count, the
// runs, the filter's checksum, and the page checksum in the last four bytes.
const (
	// headRunsAt is the offset of the first filter page run.
	headRunsAt = 6
	// headRunSize is the encoded length of a filter page run.
	headRunSize = 12
	// maxHeadRuns is the most filter page runs a head holds.
	maxHeadRuns = (pageSize - headRunsAt - 8) / headRunSize
)

// writeDelta writes the filter words and the head page of a delta built by
// out, whose tree is at root, and returns the delta.
func writeDelta(out *pageWriter, root uint64, words []uint64) (*delta, error) {
	// Write the filter a page at a time, noting the runs of pages.
	enc := make([]byte, 0, pagesFor(len(words)*8)*pageSize)
	for _, w := range words {
		enc = binary.LittleEndian.AppendUint64(enc, w)
	}
	crc := checksum(enc)
	enc = append(enc, make([]byte, cap(enc)-len(enc))...)
	var runs []run
	for off := 0; off < len(enc); off += pageSize {
		page, err := out.addRaw(enc[off : off+pageSize])
		if err != nil {
			return nil, err
		}
		if l := len(runs); l != 0 && runs[l-1].end() == page {
			runs[l-1].n++
			continue
		}
		runs = append(runs, run{start: page, n: 1})
	}
	if len(runs) > maxHeadRuns {
		return nil, errors.New("delta filter spans too many page runs")
	}

	// Encode the head locating it: the sizes, then the runs.
	b := make([]byte, pageSize)
	binary.LittleEndian.PutUint32(b, uint32(len(words)))    // #nosec G115 -- a filter is far below 1<<32 words.
	binary.LittleEndian.PutUint16(b[4:], uint16(len(runs))) // #nosec G115 -- at most maxHeadRuns.
	for i, r := range runs {
		at := headRunsAt + i*headRunSize
		binary.LittleEndian.PutUint64(b[at:], r.start)
		binary.LittleEndian.PutUint32(b[at+8:], uint32(r.n)) // #nosec G115 -- a filter run is far below 1<<32 pages.
	}

	// Check the filter and the page, and write the head.
	binary.LittleEndian.PutUint32(b[pageSize-8:], crc)
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	head, err := out.addRaw(b)
	if err != nil {
		return nil, err
	}
	return &delta{deltaRef: deltaRef{root: root, head: head}, filter: words, filterPages: runs}, nil
}

// loadDelta reads the filter of the delta ref names.
func (p *pager) loadDelta(ref deltaRef) (*delta, error) {
	// Read and check the head.
	b := make([]byte, pageSize)
	if _, err := p.r.ReadAt(b, pageOff(ref.head)); err != nil {
		return nil, err
	}
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, errors.Wrap(ErrCorrupt, "delta head checksum mismatch")
	}
	words := int(binary.LittleEndian.Uint32(b))
	nruns := int(binary.LittleEndian.Uint16(b[4:]))
	if nruns > maxHeadRuns || words == 0 {
		return nil, errors.Wrap(ErrCorrupt, "delta head out of range")
	}

	// Read the filter from its runs and check it.
	d := &delta{deltaRef: ref, filterPages: make([]run, nruns)}
	var enc []byte
	for i := range d.filterPages {
		at := headRunsAt + i*headRunSize
		r := run{start: binary.LittleEndian.Uint64(b[at:]), n: uint64(binary.LittleEndian.Uint32(b[at+8:]))}
		d.filterPages[i] = r
		start := len(enc)
		enc = append(enc, make([]byte, pageOff(r.n))...)
		if _, err := p.r.ReadAt(enc[start:], pageOff(r.start)); err != nil {
			return nil, err
		}
	}
	if len(enc) < words*8 || checksum(enc[:words*8]) != binary.LittleEndian.Uint32(b[pageSize-8:]) {
		return nil, errors.Wrap(ErrCorrupt, "delta filter checksum mismatch")
	}
	d.filter = make([]uint64, words)
	for i := range d.filter {
		d.filter[i] = binary.LittleEndian.Uint64(enc[i*8:])
	}
	return d, nil
}

// loadDeltas returns the deltas refs name, reusing those in have.
func (p *pager) loadDeltas(refs []deltaRef, have []*delta) ([]*delta, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]*delta, len(refs))
	for i, ref := range refs {
		if j := slices.IndexFunc(have, func(d *delta) bool { return d.deltaRef == ref }); j >= 0 {
			out[i] = have[j]
			continue
		}
		d, err := p.loadDelta(ref)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}

// deltaRefs returns the refs of ds, nil when there are none.
func deltaRefs(ds []*delta) []deltaRef {
	if len(ds) == 0 {
		return nil
	}
	refs := make([]deltaRef, len(ds))
	for i, d := range ds {
		refs[i] = d.deltaRef
	}
	return refs
}

// deltaPages appends to out every page d occupies: its tree, read down to the
// level above the leaves, its head, and its filter.
func (p *pager) deltaPages(out []uint64, d *delta) ([]uint64, error) {
	// Walk the tree a level at a time. Every leaf is at one depth, so one
	// read of a level's first page tells whether the level holds leaves.
	level := []uint64{d.root}
	for len(level) != 0 {
		out = append(out, level...)
		first, err := p.scanNode(level[0])
		if err != nil {
			return nil, err
		}
		if first.leaf {
			break
		}
		nodes, err := p.nodes(level)
		if err != nil {
			return nil, err
		}
		var next []uint64
		for _, n := range nodes {
			for i := range n.count() {
				next = append(next, n.kid(i))
			}
		}
		level = next
	}

	// Add the head and the filter.
	out = append(out, d.head)
	for _, r := range d.filterPages {
		out = append(out, pageRange(r.start, r.n)...)
	}
	return out, nil
}

// scanDeltas returns the entries of ds, newest first, after key after, or
// from the first key when after is nil: up to limit keys in order, each
// with the newest delta's entry, a tombstone as a delete. done reports that
// no key follows the last returned.
func (p *pager) scanDeltas(ds []*delta, after []byte, limit int) (changes []change, done bool, err error) {
	// Position a cursor on each delta past after.
	cs := make([]*cursor, len(ds))
	for i, d := range ds {
		c := &cursor{p: p, root: d.root, scan: true}
		c.seek(after, false)
		if after != nil && c.valid() && bytes.Equal(c.key(), after) {
			c.step(false)
		}
		cs[i] = c
	}

	// Take the lowest key each round; the first, newest, cursor on it wins,
	// and every cursor on it steps.
	for {
		var best []byte
		at := -1
		for i, c := range cs {
			if c.err != nil {
				return nil, false, c.err
			}
			if c.valid() && (at < 0 || bytes.Compare(c.key(), best) < 0) {
				best, at = c.key(), i
			}
		}
		if at < 0 {
			return changes, true, nil
		}
		if len(changes) == limit {
			return changes, false, nil
		}
		v := cs[at].value()
		changes = append(changes, change{key: best, del: v.del, val: v})
		for _, c := range cs {
			if c.valid() && bytes.Equal(c.key(), best) {
				c.step(false)
			}
		}
	}
}

// overChanges merges two sorted change lists, over winning on equal keys.
func overChanges(over, under []change) []change {
	out := make([]change, 0, len(over)+len(under))
	for len(over) != 0 && len(under) != 0 {
		switch c := bytes.Compare(over[0].key, under[0].key); {
		case c < 0:
			out, over = append(out, over[0]), over[1:]
		case c > 0:
			out, under = append(out, under[0]), under[1:]
		default:
			out, over, under = append(out, over[0]), over[1:], under[1:]
		}
	}
	out = append(out, over...)
	return append(out, under...)
}
