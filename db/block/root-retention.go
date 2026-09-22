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
	MarkRootsComplete(context.Context, []RootProof) error
	RootComplete(context.Context, *BlockRef, ...string) (bool, error)
}

// RootProof identifies a fully retained DAG in one decoding domain. An empty
// domain denotes a World whose normal writes supplied all dependency edges.
type RootProof struct {
	// Ref identifies the encoded root block.
	Ref *BlockRef
	// Domain separates opaque metadata from typed object traversal.
	Domain string
}

// MarkRootComplete records that a locally constructed DAG has been fenced.
// Its normal block writes must have supplied all outgoing reference metadata.
func MarkRootComplete(ctx context.Context, store StoreOps, ref *BlockRef, domain ...string) error {
	proof := RootProof{Ref: ref}
	if len(domain) != 0 {
		proof.Domain = domain[0]
	}
	return MarkRootsComplete(ctx, store, []RootProof{proof})
}

// MarkRootsComplete fences a batch of completion proofs in the owning volume.
func MarkRootsComplete(ctx context.Context, store StoreOps, proofs []RootProof) error {
	if len(proofs) == 0 || !SupportsRootRetention(store) {
		return nil
	}
	return store.(RootRetainer).MarkRootsComplete(ctx, proofs)
}

// RootComplete checks a proof whose lifetime is the physical root's lifetime.
func RootComplete(ctx context.Context, store StoreOps, ref *BlockRef, domain ...string) (bool, error) {
	if ref.GetEmpty() || !SupportsRootRetention(store) {
		return false, nil
	}
	return store.(RootRetainer).RootComplete(ctx, ref, domain...)
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
