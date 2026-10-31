package s4db

import (
	"context"

	"github.com/pkg/errors"
)

// flushJob writes the memtable of a snapshot as one level 0 run, as the
// design's Flush path describes: reserve, build off the lock, the flush
// record under it, the manifest and superblock off it, then release.
type flushJob struct {
	// db is the database.
	db *DB
}

// run flushes the memtable through the published state's seq. The caller
// holds lockFlush.
func (j *flushJob) run(ctx context.Context) error {
	// Take the snapshot: the memtable through its seq is the input, and its
	// log point is where the superblock replays the memtable from.
	db := j.db
	st, stripe := db.acquire()
	defer db.release(st, stripe)
	if st.mem.len() == 0 {
		return nil
	}

	// Reserve the run's chunks and the manifest's.
	need := chunksFor(uint64(st.mem.bytes)) + 1                                // #nosec G115 -- bytes are not negative.
	mneed := chunksFor(uint64(len(st.m.table.entries))*chunkEntrySize*5/4) + 1 // #nosec G115 -- a count.
	reserved, err := db.w.reserve(ctx, need+mneed)
	if err != nil {
		return err
	}
	runChunks, mchunks := reserved[:need], reserved[need:]

	// Build and finish the run off the lock, on the idle thread, reserving
	// more chunks when it outgrows the estimate, then flush it durably
	// through the shared flusher.
	rb := newRunBuilder(db.s, runChunks, func() (uint64, error) {
		c, err := db.w.reserve(ctx, 1)
		if err != nil {
			return 0, err
		}
		reserved = append(reserved, c[0])
		return c[0], nil
	})
	var meta runMeta
	var buildErr error
	if err := db.sched.idle.do(ctx, func() {
		st.mem.tree.Scan(func(e mentry) bool {
			buildErr = rb.add(e.key, e.val)
			return buildErr == nil
		})
		if buildErr == nil {
			meta, buildErr = rb.finish()
		}
	}); err != nil {
		buildErr = err
	}

	// From here the job finishes its records even when Close cancelled it,
	// so no reservation outlives a clean close.
	ctx = context.WithoutCancel(ctx)
	if buildErr != nil {
		return j.abandon(ctx, reserved, buildErr)
	}
	db.written.Add(rb.written)
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Append the flush record under the lock and keep the derived state.
	var next *state
	err = db.w.locked(ctx, func() error {
		// Name the run by its record and append the record.
		meta.id = (db.cur.Load().seq + 1) * 16
		meta.level = 0
		r := &record{kind: kindFlush, through: st.seq, replay: st.point(), runs: []runMeta{meta}, mchunks: mchunks}
		var err error
		next, err = db.w.appendRecord(ctx, r)
		return err
	})
	if err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Write the manifest and the superblock off the lock, then release what
	// the flush made pending.
	if err := db.publishManifest(next, mchunks); err != nil {
		return err
	}
	if err := db.w.free(ctx, rb.unused()); err != nil {
		return err
	}
	return db.sched.releaseStep(ctx)
}

// abandon frees a failed job's reservation and returns its error.
func (j *flushJob) abandon(ctx context.Context, reserved []uint64, cause error) error {
	_ = j.db.w.free(ctx, reserved)
	return cause
}

// publishManifest writes next's manifest into mchunks and flushes it, then
// writes the superblock describing next pointing at it, and marks the states
// the flush covered durable.
func (db *DB) publishManifest(next *state, mchunks []uint64) error {
	// The manifest must fit its reserved chunks, which are consecutive.
	mb := next.m.encode()
	if uint64(len(mb)) > uint64(len(mchunks))*chunkSize {
		return errors.New("manifest exceeds its reserved chunks")
	}

	// Write the manifest and flush it. The flush covers the state published
	// before it, whose records were all written by then; a commit published
	// after the flush began flushes itself.
	covered := db.cur.Load()
	off := chunkStart(mchunks[0])
	if _, err := db.s.WriteAt(mb, fileOff(off)); err != nil {
		return err
	}
	db.written.Add(int64(len(mb)))
	if err := db.s.flushDurable(); err != nil {
		return err
	}

	// Write the superblock after the manifest is durable.
	sb := next.superblock()
	sb.manifest = ref{off: off, n: uint32(len(mb)), crc: checksum(mb)} // #nosec G115 -- a manifest is far below 4 GiB.
	if err := db.writeSuper(sb); err != nil {
		return err
	}
	db.flush.setDurable(covered)
	return nil
}
