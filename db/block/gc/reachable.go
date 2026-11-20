package block_gc

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// sampleSize bounds the blocks a result lists by name for each finding.
const sampleSize = 8

// sample returns s with ref appended while s holds fewer than sampleSize.
func sample(s []*block.BlockRef, ref *block.BlockRef) []*block.BlockRef {
	if len(s) < sampleSize {
		return append(s, ref)
	}
	return s
}

// Mark is what a walk from a Volume's named roots through child lists
// reached. It is the audit of the counts and the collector: it never frees.
type Mark struct {
	// Live holds the block key of every block a root reaches.
	Live map[string]struct{}
	// Unknown counts the reached blocks whose bytes the store holds without
	// a child list. The blocks they name are not reached and may be live, so
	// a Volume with any is not adopted.
	Unknown int
	// UnknownSample names up to sampleSize of the unknown blocks.
	UnknownSample []*block.BlockRef
	// Absent counts the reached blocks the store holds neither the bytes nor
	// the child list of: holes, which name nothing a reader can reach.
	Absent int
	// AbsentRefs names every absent block a held root reaches: a root with
	// no lease, or whose lease has not lapsed.
	AbsentRefs []*block.BlockRef
	// LapsedRefs names every absent block that only roots whose lease lapsed
	// reach, such as the child of a parent a dead process's stage stored
	// ahead of it. No live holder names one. Absent counts them too.
	LapsedRefs []*block.BlockRef
}

// Has reports whether the walk reached ref.
func (m *Mark) Has(ref *block.BlockRef) bool {
	_, ok := m.Live[string(BlockKey(ref))]
	return ok
}

// StoredFunc reports whether the store holds a block's bytes.
type StoredFunc = func(ctx context.Context, ref *block.BlockRef) (bool, error)

// Reachable walks from every named root through child lists in rec, which
// should be one read snapshot, and returns what it reached. stored reports
// whether the store holds a block's bytes, for the blocks without a child
// list. It walks first from the held roots, those without a lease or whose
// lease runs past now, and then from the roots whose lease lapsed, such as
// the stages and reader pins of a process that died, so an absent block only
// the second walk reaches lands in LapsedRefs. The walk keeps the reached set
// in memory, one block key per reached block, so it suits a copy host or an
// audit, not the request path.
func Reachable(ctx context.Context, rec *Records, now time.Time, stored StoredFunc) (*Mark, error) {
	// Seed one walk with the held roots and another with the lapsed ones.
	var held, lapsed []*block.BlockRef
	leases := make(map[string]int64)
	err := rec.ScanRoots(ctx, func(_, _ string, entry *RootEntry) error {
		// Parse the block the root holds.
		ref, _, err := ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}

		// Sort it by whether its lease lapsed.
		expires, err := rec.rootExpiry(ctx, entry, leases)
		if err != nil {
			return err
		}
		if expires != 0 && expires <= now.UnixMilli() {
			lapsed = append(lapsed, ref)
			return nil
		}
		held = append(held, ref)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Walk the held roots, then what only the lapsed ones reach.
	mark := &Mark{Live: make(map[string]struct{})}
	if err := mark.walk(ctx, rec, held, stored, &mark.AbsentRefs); err != nil {
		return nil, err
	}
	if err := mark.walk(ctx, rec, lapsed, stored, &mark.LapsedRefs); err != nil {
		return nil, err
	}
	return mark, nil
}

// walk visits depth first, once each, every block stack reaches that the
// mark has not, and appends each absent block it reaches to absent.
func (m *Mark) walk(ctx context.Context, rec *Records, stack []*block.BlockRef, stored StoredFunc, absent *[]*block.BlockRef) error {
	for len(stack) != 0 {
		// Take the next unvisited block.
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		key := string(BlockKey(ref))
		if _, seen := m.Live[key]; seen {
			continue
		}
		m.Live[key] = struct{}{}
		if err := ctx.Err(); err != nil {
			return err
		}

		// Follow its child list, or classify a block without one.
		children, listed, err := rec.GetChildren(ctx, ref)
		if err != nil {
			return err
		}
		if !listed {
			if err := m.noteUnlisted(ctx, ref, stored, absent); err != nil {
				return err
			}
			continue
		}
		stack = append(stack, children...)
	}
	return nil
}

// noteUnlisted counts a reached block without a child list as unknown when
// the store holds its bytes, and as absent in absent otherwise.
func (m *Mark) noteUnlisted(ctx context.Context, ref *block.BlockRef, stored StoredFunc, absent *[]*block.BlockRef) error {
	// Count a hole.
	found, err := stored(ctx, ref)
	if err != nil {
		return err
	}
	if !found {
		m.Absent++
		*absent = append(*absent, ref)
		return nil
	}

	// Count and sample an unknown block.
	m.Unknown++
	m.UnknownSample = sample(m.UnknownSample, ref)
	return nil
}
