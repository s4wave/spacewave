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
	// log position is the superblock's memPos.
	db := j.db
	st, stripe := db.acquire()
	defer db.release(st, stripe)
	if st.mem.len() == 0 {
		return nil
	}

	// Reserve the run's chunks and the manifest's.
	need := chunksFor(uint64(st.mem.bytes)) + 1                                // #nosec G115 -- bytes are not negative.
	mneed := chunksFor(uint64(len(st.m.table.entries))*chunkEntrySize*5/4) + 1 // #nosec G115 -- a count.
	var reserved []uint64
	err := db.w.locked(ctx, func() error {
		// Allocate the chunks from a copy of the table and record them.
		next := db.cur.Load().m.table.clone()
		first := next.alloc(need+mneed, chunkReserved, db.id)
		for c := first; c < first+need+mneed; c++ {
			reserved = append(reserved, c)
		}
		_, err := db.w.appendRecord(ctx, &record{kind: kindReserve, chunks: reserved})
		return err
	})
	if err != nil {
		return err
	}
	runChunks, mchunks := reserved[:need], reserved[need:]

	// Build the run off the lock, reserving more chunks when it outgrows the
	// estimate, then flush it durably through the shared flusher.
	rb := newRunBuilder(db.s, runChunks, func() (uint64, error) {
		var c uint64
		err := db.w.locked(ctx, func() error {
			c = db.cur.Load().m.table.clone().alloc(1, chunkReserved, db.id)
			_, err := db.w.appendRecord(ctx, &record{kind: kindReserve, chunks: []uint64{c}})
			return err
		})
		return c, err
	})
	var buildErr error
	if err := db.sched.idle.do(ctx, func() {
		st.mem.tree.Scan(func(e mentry) bool {
			buildErr = rb.add(e.key, e.val)
			return buildErr == nil
		})
	}); err != nil {
		buildErr = err
	}

	// From here the job finishes its records even when Close cancelled it,
	// so no reservation outlives a clean close.
	ctx = context.WithoutCancel(ctx)
	if buildErr != nil {
		return j.abandon(ctx, reserved, buildErr)
	}
	meta, err := rb.finish()
	if err != nil {
		return j.abandon(ctx, reserved, err)
	}
	db.written.Add(rb.written)
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Append the flush record under the lock and keep the derived manifest.
	var next *state
	var sb superblock
	err = db.w.locked(ctx, func() error {
		// Name the run by its record and append the flush record.
		meta.id = (db.cur.Load().seq + 1) * 16
		meta.level = 0
		r := &record{kind: kindFlush, through: st.seq, runs: []runMeta{meta}, mchunks: mchunks}
		var err error
		next, err = db.w.appendRecord(ctx, r)
		if err != nil {
			return err
		}

		// Describe the superblock the manifest write will publish.
		sb = superblock{
			version: next.m.version,
			seq:     next.seq,
			logPos:  next.pos, logEnd: next.end, logCrc: next.crc, logWriter: next.writer,
			through: st.seq,
			memPos:  st.pos, memEnd: st.end, memCrc: st.crc, memWriter: st.writer,
		}
		return nil
	})
	if err != nil {
		return j.abandon(ctx, reserved, err)
	}

	// Write the manifest and the superblock off the lock, then release what
	// the flush made pending.
	if err := j.publishManifest(next.m, mchunks, sb); err != nil {
		return err
	}
	if unused := rb.unused(); len(unused) != 0 {
		if err := db.w.locked(ctx, func() error {
			_, err := db.w.appendRecord(ctx, &record{kind: kindFree, chunks: unused})
			return err
		}); err != nil {
			return err
		}
	}
	return db.sched.releaseStep(ctx)
}

// publishManifest writes m into mchunks and flushes it, then writes and
// flushes the superblock sb pointing at it.
func (j *flushJob) publishManifest(m *manifest, mchunks []uint64, sb superblock) error {
	// The manifest must fit its reserved chunks, which are consecutive.
	db := j.db
	mb := m.encode()
	if uint64(len(mb)) > uint64(len(mchunks))*chunkSize {
		return errors.New("manifest exceeds its reserved chunks")
	}
	off := chunkStart(mchunks[0])
	if _, err := db.s.WriteAt(mb, fileOff(off)); err != nil {
		return err
	}
	db.written.Add(int64(len(mb)))
	if err := db.s.flushDurable(); err != nil {
		return err
	}

	// Write the superblock after the manifest is durable.
	db.superMtx.Lock()
	defer db.superMtx.Unlock()
	sb.gen = db.super.gen + 1
	sb.manifest = ref{off: off, n: uint32(len(mb)), crc: checksum(mb)} // #nosec G115 -- a manifest is far below 4 GiB.
	if err := db.writeSuper(sb); err != nil {
		return err
	}
	db.super = sb
	db.flush.setDurable(db.cur.Load())
	return nil
}

// abandon frees a failed job's reservation and returns its error.
func (j *flushJob) abandon(ctx context.Context, reserved []uint64, cause error) error {
	_ = j.db.w.locked(ctx, func() error {
		_, err := j.db.w.appendRecord(ctx, &record{kind: kindFree, chunks: reserved})
		return err
	})
	return cause
}
