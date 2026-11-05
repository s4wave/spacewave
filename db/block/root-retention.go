package block

import "context"

// RootRetainer owns temporary reader pins, stages, and the staging ownership of
// roots written outside them; named roots are RootOps. Unsupported stores keep
// their existing retention policy.
type RootRetainer interface {
	// SupportsRootRetention reports whether the store owns roots. The other
	// methods require it.
	SupportsRootRetention() bool
	// PinRoot protects a root until the returned release is called.
	PinRoot(context.Context, *BlockRef) (func(), error)
	// OpenStage returns a store whose writes a new stage owns until release.
	OpenStage(context.Context) (StoreOps, func(), error)
	// ReleaseRoots drops the staging ownership of roots held by the owner of
	// this store's writes.
	ReleaseRoots(context.Context, []*BlockRef) error
	// MarkRootsComplete records that the graphs under the roots are stored.
	MarkRootsComplete(context.Context, []*BlockRef) error
	// RootsComplete reports for each root whether the graph under it is
	// recorded stored, reading all of them in one transaction.
	RootsComplete(context.Context, []*BlockRef) ([]bool, error)
}

// CompleteRootHolder is implemented by a stage store whose volume checks
// completion proofs under the stage's ownership.
type CompleteRootHolder interface {
	// HoldCompleteRoots reports which roots have a completion proof and makes
	// the stage own each of them in the same transaction.
	HoldCompleteRoots(context.Context, []*BlockRef) ([]bool, error)
}

// HoldCompleteRoots reports which roots have a completion proof and makes the
// stage own each proved root, so the graph under it survives until the stage is
// released. A copy that skips a proved subtree holds it this way until the
// parent that references it lands: a proof read without a hold can lose its
// subtree to a sweep first. stage is a store OpenStage returned. A store that
// cannot hold proofs reports none.
func HoldCompleteRoots(ctx context.Context, stage StoreOps, refs []*BlockRef) ([]bool, error) {
	holder, ok := stage.(CompleteRootHolder)
	if !ok {
		return make([]bool, len(refs)), nil
	}
	return holder.HoldCompleteRoots(ctx, refs)
}

// MarkRootComplete records that a locally constructed DAG has been fenced, or
// written ahead of the proof on a volume that makes writes durable in order.
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

// MarkDerivedRootsComplete records completion proofs for roots written over
// base, when base is empty or proven complete. A write stores only the blocks
// it changed, so the subtrees it kept are complete only where base's were: a
// device that reads base on demand from a peer holds just the changed path.
// Only a writer that stored every block its write references may derive a
// proof: a replay of another device's write may point at blocks only their
// author holds.
func MarkDerivedRootsComplete(ctx context.Context, store StoreOps, base *BlockRef, roots ...*BlockRef) error {
	if !base.GetEmpty() {
		complete, err := RootComplete(ctx, store, base)
		if err != nil || !complete {
			return err
		}
	}
	return MarkRootsComplete(ctx, store, roots)
}

// RootComplete checks a proof whose lifetime is the physical root's lifetime.
func RootComplete(ctx context.Context, store StoreOps, ref *BlockRef) (bool, error) {
	complete, err := RootsComplete(ctx, store, []*BlockRef{ref})
	if err != nil {
		return false, err
	}
	return complete[0], nil
}

// RootsComplete checks the proofs of a batch of roots in one transaction. It
// reports the proof of each root in order; an empty root has none, and a store
// without root ownership proves nothing.
func RootsComplete(ctx context.Context, store StoreOps, refs []*BlockRef) ([]bool, error) {
	if len(refs) == 0 || !SupportsRootRetention(store) {
		return make([]bool, len(refs)), nil
	}
	return store.(RootRetainer).RootsComplete(ctx, refs)
}

// SupportsRootRetention reports whether a store implements root ownership.
func SupportsRootRetention(store StoreOps) bool {
	r, ok := store.(RootRetainer)
	return ok && r.SupportsRootRetention()
}

// PinRoot protects a root until release. The caller must already hold a live
// parent or publication authority while acquiring the pin.
func PinRoot(ctx context.Context, store StoreOps, ref *BlockRef) (func(), error) {
	if ref.GetEmpty() || !SupportsRootRetention(store) {
		return func() {}, nil
	}
	return store.(RootRetainer).PinRoot(ctx, ref)
}

// OpenStage returns a store whose writes the stage owns, and the stage's
// release. The stage holds each block until a parent written outside the stage
// references it; after the release, the sweep collects every block no other
// owner holds. Hold the stage until the transaction that references the
// stage's roots returns. A store without root ownership returns itself and a
// no-op release.
func OpenStage(ctx context.Context, store StoreOps) (StoreOps, func(), error) {
	if !SupportsRootRetention(store) {
		return store, func() {}, nil
	}
	return store.(RootRetainer).OpenStage(ctx)
}

// ReleaseRoots drops the staging ownership the store holds for roots the caller
// wrote outside any named root. Each root then survives only through another
// block's reference, a named root, or a reader pin.
func ReleaseRoots(ctx context.Context, store StoreOps, refs []*BlockRef) error {
	if len(refs) == 0 || !SupportsRootRetention(store) {
		return nil
	}
	return store.(RootRetainer).ReleaseRoots(ctx, refs)
}
