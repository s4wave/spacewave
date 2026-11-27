package block

import (
	"context"

	"github.com/pkg/errors"
)

// ErrRootMoved is returned by SetRoots when a named root does not hold the
// reference its caller expected, because another writer moved it first. The
// caller reads the root again and decides whether its change still applies.
var ErrRootMoved = errors.New("named root moved")

// ErrRootsUnheld is returned by SetRoots on a store that holds no roots for a
// root that would hold a block, so a caller that needs the hold, such as a
// World whose head is a root, fails at once instead of losing the hold.
var ErrRootsUnheld = errors.New("store holds no named roots")

// ErrRootNotHeld is returned by SetRoots for a root that requires its blocks
// held when it would hold a stored block that nothing holds now: the block is
// missing, stored without a child list, at zero holders, or held only by a
// released stage. The collector may have freed what such a block references.
// The move is refused whole. A repeat of the write that stores the tree again
// in its own batch commits. A write that references blocks it did not store in
// that batch is refused again until they are stored.
var ErrRootNotHeld = errors.New("root names a block nothing holds")

// ErrUnheldPut is returned for a put that stores a block no root, parent in
// its batch, stage or recorded count holds: the collector would free it once
// its record ages past the grace period. A writer stores a block under a
// stage, or in the batch that moves the root referencing it.
var ErrUnheldPut = errors.New("put stores a block nothing holds")

// NamedRoot records a move of one named root of a store.
type NamedRoot struct {
	// Name is the root name, unique within the store's bucket.
	Name string
	// Ref is the block the name holds after the change, or empty to release
	// the name.
	Ref *BlockRef
	// Previous is the block the name must hold now, or empty when the name
	// must be free. A store that finds another reference refuses the change
	// with ErrRootMoved, so two writers never both move one root.
	Previous *BlockRef
	// Lease is the ID of the lease that holds the root, or empty for a root
	// held until released. A store that keeps leases releases every root of a
	// lease once its holder stops renewing it, so a reader pins a snapshot
	// under its process's lease, and a process that dies releases its roots
	// by silence. A store refuses a root onto a lease that is not live. A store
	// without leases holds the root until released.
	Lease string
	// RequireHeld refuses the change with ErrRootNotHeld unless the tree the
	// move newly reaches is held now: Ref itself when the change does not
	// store it, else, below each block the change stores, put anew or again,
	// every child the change does not store. A put proves a block's own bytes,
	// never its children. A writer sets it on a hold of roots it built
	// earlier, which a released stage may have left for the collector. A
	// store that frees nothing by liveness ignores it, since every block it
	// stores stays stored, and a store may be configured to only observe it.
	RequireHeld bool
}

// RootOps is the part of StoreOps that holds roots. A block is live while a
// held root reaches it through child lists, and the store decides that alone.
// A store that never frees by liveness may hold no roots: it reports none,
// and SetRoots refuses a root that would hold a block, as SetRootsUnheld
// does. Liveness frees none of its blocks, though a store may drop them by
// other means, such as a Spacewave cloud store's pack reclaim, which judges
// packs by the roots writers report to a fence. A caller that reads a root
// back needs a store that holds it.
type RootOps interface {
	// GetRoot returns the block the name holds, or nil when the name is free.
	GetRoot(ctx context.Context, name string) (*BlockRef, error)
	// ListRoots calls fn with each held root whose name starts with prefix. The
	// roots carry no Previous.
	ListRoots(ctx context.Context, prefix string, fn func(NamedRoot) error) error
	// SetRoots writes entries and moves every named root in roots as one
	// change, so a head swap stores the new pages, holds the new root and
	// releases the old one atomically. A root may hold a block entries write.
	// It writes nothing and returns ErrRootMoved when any root's Previous
	// differs from the reference the name holds. The caller serializes this
	// SetRoots call with publication of the head the root references. The
	// change is durable when SetRoots returns, together with every write the
	// store accepted before it, unless ctx asks for write ordering
	// (kvtx.WithOrderedCommitContext): a World head it publishes is
	// acknowledged to its writers as durable.
	SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error
}

// SetRootsUnheld implements SetRoots for a store that holds no roots. It
// refuses a root that would hold a block with ErrRootsUnheld, and a root whose
// Previous references a block with ErrRootMoved, since every name is free
// there. Otherwise it stores the entries. Such a store never frees a block by
// liveness, so every block written to it stays stored.
func SetRootsUnheld(ctx context.Context, store StoreOps, entries []*PutBatchEntry, roots []NamedRoot) error {
	// Refuse a hold the store cannot keep, and a swap from a block no name
	// holds.
	for _, root := range roots {
		switch {
		case !root.Ref.GetEmpty():
			return errors.Wrap(ErrRootsUnheld, root.Name)
		case !root.Previous.GetEmpty():
			return RootMoved(root.Name)
		}
	}

	// Store the entries.
	if len(entries) == 0 {
		return nil
	}
	_, err := store.PutBlockBatch(ctx, entries)
	return err
}

// RootMoved returns ErrRootMoved identifying the root.
func RootMoved(name string) error {
	return errors.Wrap(ErrRootMoved, name)
}
