package block

import "context"

// RootRetainer owns durable named roots and temporary reader pins. Replacing a
// named root releases its predecessor; immutable descendants remain protected
// by their own edges, other named roots, and reader pins. Unsupported stores
// keep their existing retention policy.
type RootRetainer interface {
	SupportsRootRetention() bool
	SetRetainedRoot(context.Context, string, *BlockRef) error
	PinRoot(context.Context, *BlockRef) (func(), error)
	MarkRootsComplete(context.Context, []*BlockRef) error
	RootComplete(context.Context, *BlockRef) (bool, error)
}

// MarkRootComplete records that a locally constructed DAG has been fenced.
// Its normal block writes must have supplied all outgoing reference metadata.
func MarkRootComplete(ctx context.Context, store StoreOps, ref *BlockRef) error {
	return MarkRootsComplete(ctx, store, []*BlockRef{ref})
}

// MarkRootsComplete fences a batch of completion proofs in the owning volume.
// Each root's writes must have supplied all outgoing reference metadata.
func MarkRootsComplete(ctx context.Context, store StoreOps, roots []*BlockRef) error {
	if len(roots) == 0 || !SupportsRootRetention(store) {
		return nil
	}
	return store.(RootRetainer).MarkRootsComplete(ctx, roots)
}

// RootComplete checks a proof whose lifetime is the physical root's lifetime.
func RootComplete(ctx context.Context, store StoreOps, ref *BlockRef) (bool, error) {
	if ref.GetEmpty() || !SupportsRootRetention(store) {
		return false, nil
	}
	return store.(RootRetainer).RootComplete(ctx, ref)
}

// SupportsRootRetention reports whether a store implements root ownership.
func SupportsRootRetention(store StoreOps) bool {
	r, ok := store.(RootRetainer)
	return ok && r.SupportsRootRetention()
}

// SetRetainedRoot advances a named root when the store supports root ownership.
// The caller serializes this operation with publication of the corresponding head.
func SetRetainedRoot(ctx context.Context, store StoreOps, name string, ref *BlockRef) error {
	if !SupportsRootRetention(store) {
		return nil
	}
	return store.(RootRetainer).SetRetainedRoot(ctx, name, ref)
}

// PinRoot protects a root until release. The caller must already hold a live
// parent or publication authority while acquiring the pin.
func PinRoot(ctx context.Context, store StoreOps, ref *BlockRef) (func(), error) {
	if ref.GetEmpty() || !SupportsRootRetention(store) {
		return func() {}, nil
	}
	return store.(RootRetainer).PinRoot(ctx, ref)
}
