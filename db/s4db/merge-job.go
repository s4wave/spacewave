package s4db

import (
	"bytes"
	"context"
)

// mergeJob combines runs level by level, as the design's Merge path
// describes: pick, reserve, merge off the lock, the merge record under it, the
// manifest and superblock off it, then release.
type mergeJob struct {
	// db is the database.
	db *DB
}

// mergePick is one chosen merge: its inputs and the level its outputs take.
type mergePick struct {
	// inputs are the runs merged, newest first.
	inputs []runMeta
	// level is the output level.
	level uint8
	// bottom reports that no run below level holds keys, so tombstones may
	// go.
	bottom bool
}

// levelBytes returns the bytes of each level's runs.
func (m *manifest) levelBytes() []int64 {
	var sizes []int64
	for _, r := range m.runs {
		for len(sizes) <= int(r.level) {
			sizes = append(sizes, 0)
		}
		sizes[r.level] += int64(len(r.chunks)) * chunkSize
	}
	return sizes
}

// overlapping returns the runs of level holding keys between lo and hi.
func (m *manifest) overlapping(level uint8, lo, hi []byte) []runMeta {
	var out []runMeta
	for _, r := range m.runs {
		if r.level == level && bytes.Compare(r.largest, lo) >= 0 && bytes.Compare(r.smallest, hi) <= 0 {
			out = append(out, r)
		}
	}
	return out
}

// pick chooses the next merge of m, or reports none. Level 0 merges whole
// into level 1 when it holds more than level0Trigger runs; otherwise the
// level furthest over its target gives its run with the least overlap below.
// A run holding the file's top chunk goes first when free chunks sit below
// it, so the end can fall. Level 0 runs overlap and are read newest first,
// so one of them never moves down alone: a level 0 run at the top takes the
// whole level with it.
func (j *mergeJob) pick(m *manifest) (mergePick, bool) {
	// Prefer the run at the top of a file with free chunks below.
	sizes := m.levelBytes()
	top, hasTop := m.topRun()
	if hasTop && top.level != 0 {
		over := m.overlapping(top.level+1, top.smallest, top.largest)
		return mergePick{inputs: append([]runMeta{top}, over...), level: top.level + 1, bottom: len(sizes) <= int(top.level)+2}, true
	}

	// Level 0 as a whole.
	if m.level0() > level0Trigger || hasTop {
		var inputs []runMeta
		var lo, hi []byte
		for _, r := range m.runs {
			if r.level != 0 {
				break
			}
			inputs = append(inputs, r)
			if lo == nil || bytes.Compare(r.smallest, lo) < 0 {
				lo = r.smallest
			}
			if hi == nil || bytes.Compare(r.largest, hi) > 0 {
				hi = r.largest
			}
		}
		inputs = append(inputs, m.overlapping(1, lo, hi)...)
		return mergePick{inputs: inputs, level: 1, bottom: len(sizes) <= 2}, true
	}

	// The level furthest over its target, by ratio.
	target := int64(runTarget)
	bestLevel, bestRatio := -1, 1.0
	for level := 1; level < len(sizes); level++ {
		target *= levelRatio
		if ratio := float64(sizes[level]) / float64(target); ratio > bestRatio {
			bestLevel, bestRatio = level, ratio
		}
	}
	if bestLevel < 0 {
		return mergePick{}, false
	}

	// Its run with the least overlapping bytes below.
	var best runMeta
	bestOverlap := int64(-1)
	var bestBelow []runMeta
	for _, r := range m.runs {
		if int(r.level) != bestLevel {
			continue
		}
		below := m.overlapping(r.level+1, r.smallest, r.largest)
		var overlap int64
		for _, b := range below {
			overlap += int64(len(b.chunks)) * chunkSize
		}
		if bestOverlap < 0 || overlap < bestOverlap {
			best, bestOverlap, bestBelow = r, overlap, below
		}
	}
	return mergePick{inputs: append([]runMeta{best}, bestBelow...), level: best.level + 1, bottom: len(sizes) <= bestLevel+2}, true
}

// topRun returns the run holding the file's last chunk when free chunks sit
// below it.
func (m *manifest) topRun() (runMeta, bool) {
	// The last chunk must be a run chunk with a free chunk below it.
	t := m.table
	end := t.end()
	if end <= firstChunk || t.entries[end-1].kind != chunkRun {
		return runMeta{}, false
	}
	free := false
	for c := uint64(firstChunk); c < end-1 && !free; c++ {
		free = t.entries[c].kind == chunkFree
	}
	if !free {
		return runMeta{}, false
	}

	// Find the run.
	id := t.entries[end-1].owner
	for _, r := range m.runs {
		if r.id == id {
			return r, true
		}
	}
	return runMeta{}, false
}

// run performs one merge. The caller holds lockMerge.
func (j *mergeJob) run(ctx context.Context) error {
	// Choose the merge on a snapshot.
	db := j.db
	st, stripe := db.acquire()
	defer db.release(st, stripe)
	pick, ok := j.pick(st.m)
	if !ok {
		return nil
	}

	// Reserve the output chunks and the manifest's.
	var inBytes uint64
	for _, r := range pick.inputs {
		inBytes += uint64(len(r.chunks)) * chunkSize
	}
	need := chunksFor(inBytes) + 1
	mneed := chunksFor(uint64(len(st.m.table.entries))*chunkEntrySize*5/4) + 1 // #nosec G115 -- a count.
	reserved, err := db.w.reserve(ctx, need+mneed)
	if err != nil {
		return err
	}
	runChunks, mchunks := reserved[:need], reserved[need:]

	// Merge the inputs into output runs off the lock, on the idle thread,
	// reserving more chunks when the outputs outgrow the estimate.
	more := func() (uint64, error) {
		c, err := db.w.reserve(ctx, 1)
		if err != nil {
			return 0, err
		}
		reserved = append(reserved, c[0])
		return c[0], nil
	}
	var outputs []runMeta
	var unused []uint64
	var mergeErr error
	if err := db.sched.idle.do(ctx, func() {
		outputs, unused, mergeErr = j.merge(pick, runChunks, more)
	}); err != nil {
		mergeErr = err
	}

	// From here the job finishes its records even when Close cancelled it,
	// so no reservation outlives a clean close.
	ctx = context.WithoutCancel(ctx)
	if mergeErr != nil {
		return j.abandon(ctx, reserved, mergeErr)
	}
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Append the merge record under the lock and keep the derived manifest.
	removed := make([]uint64, len(pick.inputs))
	for i, r := range pick.inputs {
		removed[i] = r.id
	}
	var next *state
	err = db.w.locked(ctx, func() error {
		// Name the outputs by the record and append it.
		seq := db.cur.Load().seq + 1
		for i := range outputs {
			outputs[i].id = seq*16 + uint64(i) // #nosec G115 -- few outputs.
			outputs[i].level = pick.level
		}
		var err error
		next, err = db.w.appendRecord(ctx, &record{kind: kindMerge, removed: removed, runs: outputs, mchunks: mchunks})
		return err
	})
	if err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Write the manifest and superblock, forget the inputs, free the unused
	// reservation, and release.
	if err := db.publishManifest(next, mchunks); err != nil {
		return err
	}
	for _, id := range removed {
		db.forgetRun(id)
	}
	if err := db.w.free(ctx, unused); err != nil {
		return err
	}
	return db.sched.releaseStep(ctx)
}

// merge reads the inputs through a merge of their layers, newest first, and
// writes output runs of runTarget into chunks, taking more when they run
// out, returning the runs and the chunks left unused.
func (j *mergeJob) merge(pick mergePick, chunks []uint64, more func() (uint64, error)) ([]runMeta, []uint64, error) {
	// Open a layer per input, newest first.
	layers := make([]layer, 0, len(pick.inputs))
	for _, r := range pick.inputs {
		layers = append(layers, &runIter{db: j.db, meta: r})
	}
	for _, l := range layers {
		l.seek(nil, false)
	}

	// Write entries in order into runs of runTarget each; each run takes the
	// chunks the one before it left.
	var outputs []runMeta
	var rb *runBuilder
	for {
		// Find the smallest key and the newest layer holding it.
		var best []byte
		var ent ientry
		for _, l := range layers {
			if !l.valid() {
				continue
			}
			if best == nil || bytes.Compare(l.key(), best) < 0 {
				best, ent = l.key(), l.entry()
			}
		}
		if best == nil {
			break
		}
		key := bytes.Clone(best)
		for _, l := range layers {
			if l.valid() && bytes.Equal(l.key(), key) {
				l.step(false)
			}
		}
		if err := j.checkLayers(layers); err != nil {
			return nil, nil, err
		}

		// Drop a tombstone at the bottom; write the rest.
		if ent.del && pick.bottom {
			continue
		}
		if rb == nil {
			rb = newRunBuilder(j.db.s, chunks, more)
		}
		if err := rb.add(key, ent.val); err != nil {
			return nil, nil, err
		}
		if rb.written+int64(len(rb.buf)) >= runTarget {
			meta, err := rb.finish()
			if err != nil {
				return nil, nil, err
			}
			outputs = append(outputs, meta)
			chunks = rb.unused()
			j.db.written.Add(rb.written)
			rb = nil
		}
	}

	// Finish the last run.
	if rb != nil && rb.count != 0 {
		meta, err := rb.finish()
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, meta)
		chunks = rb.unused()
		j.db.written.Add(rb.written)
	}
	return outputs, chunks, nil
}

// checkLayers returns the first error a run layer hit.
func (j *mergeJob) checkLayers(layers []layer) error {
	for _, l := range layers {
		if it, ok := l.(*runIter); ok && it.err != nil {
			return it.err
		}
	}
	return nil
}

// abandon frees a failed job's reservation and returns its error.
func (j *mergeJob) abandon(ctx context.Context, reserved []uint64, cause error) error {
	_ = j.db.w.free(ctx, reserved)
	return cause
}
