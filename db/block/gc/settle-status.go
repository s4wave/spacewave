package block_gc

import (
	"bytes"
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
	// FrontCauses counts the front entries read by their detailed proof, with one ref per cause.
	FrontCauses [frontCauses]FrontCauseCount
	// Holes counts the recorded holes, up to the limit, written off or not.
	Holes int
	// OpenHoles counts the holes among Holes no operator wrote off, which
	// are the ones that hold the flip.
	OpenHoles int
	// HolesMore reports that more holes are recorded than Holes.
	HolesMore bool
	// HoleSample holds the first open holes' blocks.
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
// limit keys of each of two prefix scans, the front proof reads per front entry,
// and one queue seek.
func (c *Collector) SettleStatus(ctx context.Context, limit int) (SettleStatus, error) {
	var st SettleStatus
	err := c.store.ViewBlocks(ctx, func(rec *Records, blocks block.StoreOps) error {
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
		if err = st.readFront(ctx, rec, blocks, limit); err != nil {
			return err
		}
		if err = st.readHoles(ctx, rec, limit); err != nil {
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

// readHoles counts the recorded holes up to limit and the open ones among
// them, and samples the first open holes' blocks, since those are the ones
// a put must store again.
func (s *SettleStatus) readHoles(ctx context.Context, rec *Records, limit int) error {
	return rec.scan(ctx, rec.key(kindHole), nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Stop past the limit.
		if s.Holes == limit {
			s.HolesMore = true
			return false, nil
		}
		s.Holes++

		// Count and sample an open hole.
		v, err := value()
		if err != nil || bytes.Equal(v, holeWrittenOff) {
			return err == nil, err
		}
		s.OpenHoles++
		if len(s.HoleSample) < sampleSize {
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return false, err
			}
			s.HoleSample = append(s.HoleSample, ref)
		}
		return true, nil
	})
}

// readFront counts and classifies each front entry in the same bounded scan.
func (s *SettleStatus) readFront(ctx context.Context, rec *Records, blocks block.StoreOps, limit int) error {
	return rec.scan(ctx, rec.key(kindFront), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Stop before reading proof records for entries beyond the limit.
		if s.Front == limit {
			s.FrontMore = true
			return false, nil
		}
		cause, _, err := readFrontCause(ctx, rec, blocks, s.State.GetUpCursor(), key)
		if err != nil {
			return false, err
		}

		// Retain the first ref per cause and the existing bounded front sample.
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return false, err
		}
		s.Front++
		count := &s.FrontCauses[cause]
		count.Count++
		if count.Sample == nil {
			count.Sample = ref
		}
		if len(s.FrontSample) < sampleSize {
			s.FrontSample = append(s.FrontSample, ref)
		}
		return true, nil
	})
}
