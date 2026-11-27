package block_gc

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// SettleStatus is what the flip's checks find in one read snapshot, as
// Collector.flip takes them, so a caller can tell why an Open generation has
// not flipped. It writes nothing.
type SettleStatus struct {
	// State is the liveness state.
	State *LivenessState
	// Freeable reports that the collector's free may run: the Volume is
	// adopted, the grace period since its adoption passed, and no paged
	// release is in progress, as Collector.freeable describes.
	Freeable bool
	// GraceLeft is the time left of the grace period since the adoption, zero
	// once it passed.
	GraceLeft time.Duration
	// Rewritten reports that a list rewrite left the counts owing a recount,
	// which returns the Volume to unadopted at the flip, as kindRewritten
	// describes.
	Rewritten bool
	// RunComplete reports that the unmarked run's completion entry exists.
	RunComplete bool
	// Front counts the front's entries, up to the limit.
	Front int
	// FrontMore reports that the front holds more entries than Front.
	FrontMore bool
	// FrontSample holds the first front entries' blocks.
	FrontSample []*block.BlockRef
	// Holes counts the recorded holes, up to the limit.
	Holes int
	// HolesMore reports that more holes are recorded than Holes.
	HolesMore bool
	// HoleSample holds the first recorded holes' blocks.
	HoleSample []*block.BlockRef
	// RipeQueued reports that a block queued to free at or below the pop
	// mark and the up cursor is not parked yet. That queued block holds the
	// flip until a free parks the block.
	RipeQueued bool
}

// SettleStatus reads what the flip's checks find, counting the front and
// the holes up to limit each, with a sample of each.
//
// Cost: one read snapshot holding the state read, two point reads, at most
// limit keys of each of two prefix scans, and one queue seek.
func (c *Collector) SettleStatus(ctx context.Context, limit int) (SettleStatus, error) {
	var st SettleStatus
	err := c.store.View(ctx, func(rec *Records) error {
		// Read the state and the freeable clauses.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		st = SettleStatus{State: state, Freeable: c.freeable(state)}
		adopted := time.UnixMilli(state.GetAdoptAtUnixMilli())
		st.GraceLeft = max(c.opts.Grace-c.opts.Now().Sub(adopted), 0)

		// Read the rewrite marker and the run's completion entry.
		if st.Rewritten, err = rec.recountOwed(ctx); err != nil {
			return err
		}
		if st.RunComplete, err = rec.tx.Exists(ctx, rec.key(kindUnmarked, runComplete)); err != nil {
			return err
		}

		// Count the front and the holes.
		if st.Front, st.FrontMore, st.FrontSample, err = rec.countKeys(ctx, kindFront, limit); err != nil {
			return err
		}
		if st.Holes, st.HolesMore, st.HoleSample, err = rec.countKeys(ctx, kindHole, limit); err != nil {
			return err
		}

		// Check for a ripe queued block, as the flip does once the front
		// popped any.
		mark, popped, err := rec.popMark(ctx)
		if err != nil || !popped {
			return err
		}
		st.RipeQueued, _, err = rec.ripeQueued(ctx, nil, min(mark, state.GetUpCursor()))
		return err
	})
	return st, err
}

// countKeys counts the block-keyed records of kind up to limit, reports
// whether more exist, and returns the first sampleSize blocks.
func (r *Records) countKeys(ctx context.Context, kind byte, limit int) (int, bool, []*block.BlockRef, error) {
	// Scan the kind's keys up to the limit.
	var n int
	var more bool
	var sample []*block.BlockRef
	err := r.scan(ctx, r.key(kind), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Stop past the limit.
		if n == limit {
			more = true
			return false, nil
		}
		n++

		// Sample the first blocks.
		if len(sample) < sampleSize {
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return false, err
			}
			sample = append(sample, ref)
		}
		return true, nil
	})
	return n, more, sample, err
}
