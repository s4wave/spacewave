package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// stagePageView stores a staged build together with any kept World pages it
// reuses. A physical staged parent must never depend on engine-only bytes:
// the existing stage owns those bytes until adoption or abandonment, even if
// replay sweeps the parent's temporary tracking entry before publication.
type stagePageView struct {
	// pageView serves kept reads and tracks the physical stage's writes.
	pageView
}

// PutBlock carries reused kept descendants in the same staged batch as their
// parent, preserving its selected hash, forced reference and durability fence.
func (v *stagePageView) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Leaf writes and already-physical dependencies keep the original put path.
	carried := v.set.entriesOf(opts.GetRefs())
	if len(carried) == 0 {
		return v.pageView.PutBlock(ctx, data, opts)
	}

	// Select exactly the reference the underlying single put would produce.
	if err := opts.Validate(); err != nil {
		return nil, false, err
	}
	ref, err := block.BuildBlockRef(data, &block.PutOpts{HashType: opts.SelectHashType(v.GetHashType())})
	if err != nil {
		return nil, false, err
	}
	if forced := opts.GetForceBlockRef(); !forced.GetEmpty() && !forced.EqualsRef(ref) {
		return nil, false, block.ErrBlockRefMismatch
	}

	// An open stage owns every write, including cache fills, until release.
	// Put the parent and its dependencies together so ordinary edges fund once.
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	found, err := v.putWithKept(ctx, []*block.PutBatchEntry{entry}, carried)
	if err != nil {
		return nil, false, err
	}
	if opts.GetSync() {
		if _, err := v.Sync(ctx); err != nil {
			return nil, false, err
		}
	}
	return ref, found[0], nil
}

// PutBlockBatch carries reused kept descendants in the same physical batch.
// Only the caller's entries contribute to the returned existence results.
func (v *stagePageView) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Snapshot the bounded kept dependencies before replay can sweep the index.
	var refs []*block.BlockRef
	for _, entry := range entries {
		refs = append(refs, entry.Refs...)
	}
	return v.putWithKept(ctx, entries, v.set.entriesOf(refs))
}

// putWithKept prepends immutable dependency bytes to their real parent batch.
// A standalone dependency re-put could add a surplus holder in a counted store.
func (v *stagePageView) putWithKept(ctx context.Context, entries, kept []*block.PutBatchEntry) ([]bool, error) {
	// Avoid rewriting a dependency already supplied by the caller.
	present := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		present[entry.Ref.MarshalString()] = struct{}{}
	}
	var carried []*block.PutBatchEntry
	for _, entry := range kept {
		if _, ok := present[entry.Ref.MarshalString()]; !ok {
			carried = append(carried, entry)
		}
	}

	// Preserve one stage transaction and the caller's result indices.
	found, err := v.pageView.PutBlockBatch(ctx, append(carried, entries...))
	if err != nil {
		return nil, err
	}
	return found[len(carried):], nil
}
