package block

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// ErrMissingChild refuses a write to the inner store of a block that names a
// child no store holds: written beside it, kept, or stored. Storing it would
// leave a stored World that cannot be read; the refused write moves nothing,
// so the roots keep naming what they held.
var ErrMissingChild = errors.New("block names a child that is neither written, kept nor stored")

// MissingChildError is the refusal of one write: Op names the write, Block the
// block it refused and Child the child of Block no store holds. It matches
// ErrMissingChild.
type MissingChildError struct {
	// Op names the refused write, such as a Flush or a SetRoots.
	Op string
	// Block is the refused block.
	Block *BlockRef
	// Child is the child of Block no store holds.
	Child *BlockRef
}

// Error names the write, the block and its child.
func (e *MissingChildError) Error() string {
	return e.Op + ": block " + e.Block.MarshalString() + " names " + e.Child.MarshalString() + ": " + ErrMissingChild.Error()
}

// Is reports whether target is ErrMissingChild.
func (e *MissingChildError) Is(target error) bool {
	return target == ErrMissingChild
}

// LocalStoreGetter is a store whose blocks live in tiers: the local store the
// local collector frees from, and sources behind it such as a storage
// backend, the Cloud or peers.
type LocalStoreGetter interface {
	// GetLocalStore returns the local tier, or nil when the store has none.
	GetLocalStore() StoreOps
}

// checkChildren refuses batch with a MissingChildError, naming op, the block and
// its child by their block references, when a block of batch names a child
// that is neither in batch, kept, held already (PutBatchEntry.Held), nor in
// the inner store. A kept child is either written by this write's caller or
// stored by an open Flush.
//
// Invariant: the check refuses only on proof of absence. It never refuses a
// write for a child it could not look for, and it adds no network round trip
// to a write whose children are local. It asks the inner store's local tier
// (LocalStoreGetter) first; only the children that tier lacks go to the whole
// store, once, under WithoutPeerWait. A source there that cannot look or
// cannot know, such as a backend not yet open or peers that may not be every
// device holding the block, answers ErrUnavailable,
// and any failure of that probe admits the write as unchecked (Unchecked)
// rather than refuse it. A store with no local tier answers definitely.
//
// Cost: one local existence read of every distinct child the batch names that
// it neither writes, keeps nor holds already, the frontier between the kept
// pages and the stored ones, and one read of the whole store for the children
// the local tier lacks, which waits at most childProbeWait.
func (s *PendingStore) checkChildren(ctx context.Context, op string, batch []*PutBatchEntry) error {
	// Gather the children neither written with the batch nor kept.
	seen := make(map[string]struct{}, len(batch))
	for _, e := range batch {
		seen[pendingKey(e.Ref)] = struct{}{}
		for _, ref := range e.Held {
			seen[pendingKey(ref)] = struct{}{}
		}
	}
	var children, parents []*BlockRef
	s.mtx.RLock()
	for _, e := range batch {
		for _, ref := range e.Refs {
			if ref.GetEmpty() {
				continue
			}
			key := pendingKey(ref)
			if _, ok := seen[key]; ok || s.kept[key] != nil {
				continue
			}
			seen[key] = struct{}{}
			children, parents = append(children, ref), append(parents, e.Ref)
		}
	}
	s.mtx.RUnlock()

	// Pass the children the local tier holds.
	if len(children) == 0 {
		return nil
	}
	local := s.StoreOps
	if tiers, ok := s.StoreOps.(LocalStoreGetter); ok && tiers.GetLocalStore() != nil {
		local = tiers.GetLocalStore()
	}
	found, err := local.GetBlockExistsBatch(ctx, children)
	if err != nil {
		return err
	}
	children, parents = missedChildren(found, children, parents)
	if len(children) == 0 || local == s.StoreOps {
		return refuseMissing(op, children, parents)
	}

	// Ask the whole store about the rest once, without waiting for a peer
	// and for at most childProbeWait; a probe that could not look in time
	// admits the write unchecked.
	probe, cancel := childProbe(ctx)
	found, err = s.StoreOps.GetBlockExistsBatch(probe, children)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		s.unchecked.Add(uint64(len(children)))
		if report := s.uncheckedReport; report != nil {
			report(op, len(children), err)
		}
		return nil
	}
	children, parents = missedChildren(found, children, parents)
	return refuseMissing(op, children, parents)
}

// childProbeWait bounds a whole-store existence probe made for a write. A
// writer may hold a turn or a root set's lock behind the probe, and the probe
// may ask peers and a remote backend, so it reports unknown rather than wait
// past the bound.
const childProbeWait = 500 * time.Millisecond

// childProbe returns the context of a whole-store existence probe made for a
// write under ctx: it never waits for a peer to connect and ends after
// childProbeWait.
func childProbe(ctx context.Context) (context.Context, context.CancelFunc) {
	probe, _ := WithoutPeerWait(ctx)
	return context.WithTimeoutCause(probe, childProbeWait, ErrUnavailable)
}

// missedChildren returns the children found reports absent, with their parents.
func missedChildren(found []bool, children, parents []*BlockRef) ([]*BlockRef, []*BlockRef) {
	var missed, missedParents []*BlockRef
	for i, ok := range found {
		if !ok {
			missed, missedParents = append(missed, children[i]), append(missedParents, parents[i])
		}
	}
	return missed, missedParents
}

// refuseMissing refuses op over the first proven-absent child, if any.
func refuseMissing(op string, children, parents []*BlockRef) error {
	if len(children) == 0 {
		return nil
	}
	return &MissingChildError{Op: op, Block: parents[0], Child: children[0]}
}

// Unchecked returns how many children a write named that the child check
// could not look for, and so admitted without proof that they exist.
func (s *PendingStore) Unchecked() uint64 {
	return s.unchecked.Load()
}

// SetUncheckedReport sets the function told of each write the child check
// admitted unchecked: the write, how many children it could not look for, and
// why. Call it before the store is shared.
func (s *PendingStore) SetUncheckedReport(report func(op string, children int, err error)) {
	s.uncheckedReport = report
}
