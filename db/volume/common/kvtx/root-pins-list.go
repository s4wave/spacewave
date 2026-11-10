package kvtx

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// RootPin is a root this process holds outside every named root: the root a
// reader pinned, or a block an open stage owns.
type RootPin struct {
	// Root is the pinned root or the stage's block.
	Root *block.BlockRef
	// Stage is the owning stage's node, or empty for a reader pin.
	Stage string
	// Opened is when the pin took its first reader, or the stage opened.
	Opened time.Time
}

// ListRootPins returns the roots this process holds outside every named root:
// each root a reader holds pinned, and each block an open stage owns directly.
// A mark that starts from a World head adds these to the head's roots, since
// the head names neither. A released pin or stage is not listed, even while
// its edge waits for removal. The list is a snapshot; pins and stages opened
// after it are newer than it.
func (v *Volume) ListRootPins(ctx context.Context) ([]RootPin, error) {
	// Copy the held pins and the open stages under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	var out []RootPin
	for node, pin := range v.rootPins {
		ref, ok := block_gc.ParseBlockIRI(node)
		if ok && pin.count > 0 {
			out = append(out, RootPin{Root: ref, Opened: pin.opened})
		}
	}
	stages := make(map[string]time.Time, len(v.stages))
	for stage, st := range v.stages {
		if !st.released {
			stages[stage] = st.opened
		}
	}
	unlock()

	// Read the blocks each stage owns from its edges.
	for stage, opened := range stages {
		nodes, err := v.refGraph.GetOutgoingRefs(ctx, stage)
		if err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if ref, ok := block_gc.ParseBlockIRI(node); ok {
				out = append(out, RootPin{Root: ref, Stage: stage, Opened: opened})
			}
		}
	}

	// Order the list by open time, then by stage and root.
	slices.SortFunc(out, func(a, b RootPin) int {
		if c := a.Opened.Compare(b.Opened); c != 0 {
			return c
		}
		if c := strings.Compare(a.Stage, b.Stage); c != 0 {
			return c
		}
		return strings.Compare(a.Root.MarshalString(), b.Root.MarshalString())
	})
	return out, nil
}
