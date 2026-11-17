package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
)

// knownAbsent is the set of World blocks the replica copy found that no source
// serves, carried from one head to the next so each walk records them as holes
// again without asking the peers for each.
//
// Invariant: a ref is in the set only if a read through every tier, peers
// included, found it absent after the last peer session start and under the
// current authority config. Only a new peer session or a config change can make
// such a block servable, and each one resets the set, which advances its
// generation; a walk or probe adds refs only under the generation it started
// in, so a ref read before a reset never survives it. A local write of the
// same block is not watched: the next reset reads the block again.
//
// The set is an optimization for reads only. It never causes a write, a
// delete, or a decision that a block is gone for good: a ref in the set is
// still reported as a hole on every walk, and is read again after the next
// reset; it is retried, never abandoned.
type knownAbsent struct {
	// bcast guards the fields below and wakes waitReset.
	bcast broadcast.Broadcast
	// gen counts the resets.
	gen uint64
	// refs holds the absent refs by their marshaled key.
	refs map[string]struct{}
}

// generation returns the count of resets so far.
func (k *knownAbsent) generation() uint64 {
	var gen uint64
	k.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		gen = k.gen
	})
	return gen
}

// has reports whether ref is known absent.
func (k *knownAbsent) has(ref *block.BlockRef) bool {
	var found bool
	k.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, found = k.refs[ref.MarshalString()]
	})
	return found
}

// add records ref as absent if no reset happened since gen.
func (k *knownAbsent) add(gen uint64, ref *block.BlockRef) {
	k.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if k.gen != gen {
			return
		}
		if k.refs == nil {
			k.refs = make(map[string]struct{})
		}
		k.refs[ref.MarshalString()] = struct{}{}
	})
}

// replace makes refs the whole set if no reset happened since gen, dropping the
// refs a newer head no longer reaches.
func (k *knownAbsent) replace(gen uint64, refs []*block.BlockRef) {
	k.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if k.gen != gen {
			return
		}
		k.refs = make(map[string]struct{}, len(refs))
		for _, ref := range refs {
			k.refs[ref.MarshalString()] = struct{}{}
		}
	})
}

// reset empties the set and wakes waitReset, when a source that may serve its
// blocks appears.
func (k *knownAbsent) reset() {
	k.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		k.gen++
		k.refs = nil
		bcast()
	})
}

// waitReset waits until a reset happens after generation returned gen.
func (k *knownAbsent) waitReset(ctx context.Context, gen uint64) error {
	return k.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return k.gen != gen, nil
	})
}

// resetOnSessionStart resets absent each time a peer session starts on
// exchange after GetSessionStarts returned starts, until ctx ends. The caller
// reads starts before any walk begins, so no session start falls between a
// walk and the watch.
func resetOnSessionStart(ctx context.Context, exchange *dex_solicit.Controller, starts uint64, absent *knownAbsent) {
	for exchange.WaitSessionStart(ctx, starts) == nil {
		starts = exchange.GetSessionStarts()
		absent.reset()
	}
}

// absentSource reads through its store, but answers a known-absent ref as not
// found without asking the store, so the graph copy records it as a hole again
// without a peer read.
type absentSource struct {
	block.StoreOps
	// absent is the set of refs answered as not found.
	absent *knownAbsent
}

// GetStoredBlock reads ref unless it is known absent.
func (s *absentSource) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if s.absent.has(ref) {
		return nil, block.NotFoundError(ref)
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// GetCopySource returns the store's own copy source behind the same set, so a
// copy keeps reading without filling the cache.
func (s *absentSource) GetCopySource() block.StoreOps {
	src := s.StoreOps
	if cs, ok := src.(block.CopySource); ok {
		src = cs.GetCopySource()
	}
	return &absentSource{StoreOps: src, absent: s.absent}
}
