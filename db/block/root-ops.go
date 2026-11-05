package block

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// ErrRootMoved is returned by SetRoots when a named root does not hold the
// reference its caller expected, because another writer moved it first. The
// caller reads the root again and decides whether its change still applies.
var ErrRootMoved = errors.New("named root moved")

// NamedRoot moves one named root of a store.
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
	// Expires is when the store releases the root itself, or the zero time
	// for a root held until released. A holder renews a leased root by setting it again with the
	// same Ref and Previous and a later Expires, so a reader pins a snapshot
	// by holding a root it renews, and a holder that dies releases it by
	// silence.
	Expires time.Time
}

// RootOps is the part of StoreOps that holds roots. A block is live while a
// held root reaches it through child lists, and the store decides that alone.
// A store that never frees by liveness holds no roots: it reports none and
// SetRoots only stores the entries, as SetRootsUnheld does. Dropping a root
// there loses nothing, since such a store keeps every block it stores.
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
	// with publication of the head the root names.
	SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error
}

// SetRootsUnheld implements SetRoots for a store that holds no roots: it
// stores the entries and drops the roots. Such a store never frees a block by
// liveness, so every block written to it stays stored.
func SetRootsUnheld(ctx context.Context, store StoreOps, entries []*PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	_, err := store.PutBlockBatch(ctx, entries)
	return err
}

// RootMoved returns ErrRootMoved naming the root.
func RootMoved(name string) error {
	return errors.Wrap(ErrRootMoved, name)
}

// OverwriteRoots writes entries and sets each named root to its Ref whatever
// it held, for a caller that is the only writer of those names and keeps no
// copy of their references. It reads the roots and swaps them, again if
// another writer moved one between the two. It replaces any Previous in roots.
//
// It exists only for five callers that never tracked their root: the account
// World copy, the legacy World publication, the World engine controller, the
// storage reclaim fence and the replay root hold. It is deleted when they are.
func OverwriteRoots(ctx context.Context, store StoreOps, entries []*PutBatchEntry, roots []NamedRoot) error {
	checked := make([]NamedRoot, len(roots))
	for {
		for i, root := range roots {
			held, err := store.GetRoot(ctx, root.Name)
			if err != nil {
				return err
			}
			root.Previous = held
			checked[i] = root
		}
		err := store.SetRoots(ctx, entries, checked)
		if !errors.Is(err, ErrRootMoved) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
