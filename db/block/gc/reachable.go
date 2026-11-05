package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// unknownSampleSize bounds the unknown blocks a Mark lists by name.
const unknownSampleSize = 8

// Mark is what a walk from a Volume's named roots through child lists
// reached. It is the audit of the counts and the collector: it never frees.
type Mark struct {
	// Live holds the block key of every block a root reaches.
	Live map[string]struct{}
	// Unknown counts the reached blocks whose bytes the store holds without
	// a child list. The blocks they name are not reached and may be live, so
	// a Volume with any is not adopted.
	Unknown int
	// UnknownSample names up to unknownSampleSize of the unknown blocks.
	UnknownSample []*block.BlockRef
	// Absent counts the reached blocks the store holds neither the bytes nor
	// the child list of: holes, which name nothing a reader can reach.
	Absent int
	// AbsentRefs names every absent block.
	AbsentRefs []*block.BlockRef
}

// Has reports whether the walk reached ref.
func (m *Mark) Has(ref *block.BlockRef) bool {
	_, ok := m.Live[string(BlockKey(ref))]
	return ok
}

// Reachable walks from every named root through child lists in rec, which
// should be one read snapshot, and returns what it reached. stored reports
// whether the store holds a block's bytes, for the blocks without a child
// list. The walk keeps the reached set in memory, one block key per reached
// block, so it suits a copy host or an audit, not the request path.
func Reachable(ctx context.Context, rec *Records, stored func(ctx context.Context, ref *block.BlockRef) (bool, error)) (*Mark, error) {
	// Seed the walk with every root.
	mark := &Mark{Live: make(map[string]struct{})}
	var stack []*block.BlockRef
	err := rec.ScanRoots(ctx, func(_, _ string, entry *RootEntry) error {
		ref, _, err := ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}
		stack = append(stack, ref)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Walk depth first, visiting each block once.
	for len(stack) != 0 {
		// Take the next unvisited block.
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		key := string(BlockKey(ref))
		if _, seen := mark.Live[key]; seen {
			continue
		}
		mark.Live[key] = struct{}{}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Follow its child list, or classify a block without one.
		children, listed, err := rec.GetChildren(ctx, ref)
		if err != nil {
			return nil, err
		}
		if !listed {
			if err := mark.noteUnlisted(ctx, ref, stored); err != nil {
				return nil, err
			}
			continue
		}
		stack = append(stack, children...)
	}
	return mark, nil
}

// noteUnlisted counts a reached block without a child list as unknown when
// the store holds its bytes, and as absent otherwise.
func (m *Mark) noteUnlisted(ctx context.Context, ref *block.BlockRef, stored func(ctx context.Context, ref *block.BlockRef) (bool, error)) error {
	// Count a hole.
	found, err := stored(ctx, ref)
	if err != nil {
		return err
	}
	if !found {
		m.Absent++
		m.AbsentRefs = append(m.AbsentRefs, ref)
		return nil
	}

	// Count and sample an unknown block.
	m.Unknown++
	if len(m.UnknownSample) < unknownSampleSize {
		m.UnknownSample = append(m.UnknownSample, ref)
	}
	return nil
}
