package block

import "context"

// RootProofs keeps a store's completion proofs: records that the graph under
// a root is stored, which a graph copy reads to skip the subtree. A proof is
// derived data and holds nothing. The collector deletes it with its block, so
// a copy that skips a proved subtree holds it under a stage first, and a false
// proof hides the blocks under it from every later copy.
type RootProofs interface {
	// MarkRootsComplete records that the graphs under the roots are stored.
	MarkRootsComplete(context.Context, []*BlockRef) error
	// RootsComplete reports for each root whether the graph under it is
	// recorded stored, reading all of them in one transaction.
	RootsComplete(context.Context, []*BlockRef) ([]bool, error)
	// HoldCompleteRoots reports which roots have a proof and makes the stage
	// hold each of them in the same transaction, on a store OpenStage
	// returned. Any other store reports none.
	HoldCompleteRoots(context.Context, []*BlockRef) ([]bool, error)
}

// rootProofs returns the proofs of a store that holds roots, or nil.
func rootProofs(store StoreOps) RootProofs {
	p, ok := store.(RootProofs)
	if !ok || !HoldsRoots(store) {
		return nil
	}
	return p
}

// HoldCompleteRoots reports which roots have a completion proof and makes the
// stage hold each proved root, so the graph under it survives until the stage
// is released. A copy that skips a proved subtree holds it this way until the
// parent that references it lands: a proof read without a hold can lose its
// subtree to the collector first. stage is a store OpenStage returned. A store
// that keeps no proofs reports none.
func HoldCompleteRoots(ctx context.Context, stage StoreOps, refs []*BlockRef) ([]bool, error) {
	p := rootProofs(stage)
	if p == nil {
		return make([]bool, len(refs)), nil
	}
	return p.HoldCompleteRoots(ctx, refs)
}

// MarkRootComplete records that a locally constructed DAG has been fenced, or
// written ahead of the proof on a volume that makes writes durable in order.
// Its normal block writes must have supplied all outgoing reference metadata.
func MarkRootComplete(ctx context.Context, store StoreOps, ref *BlockRef) error {
	return MarkRootsComplete(ctx, store, []*BlockRef{ref})
}

// MarkRootsComplete records the completion proofs of a batch of roots. Each
// root's writes must have supplied all outgoing reference metadata.
func MarkRootsComplete(ctx context.Context, store StoreOps, roots []*BlockRef) error {
	p := rootProofs(store)
	if len(roots) == 0 || p == nil {
		return nil
	}
	return p.MarkRootsComplete(ctx, roots)
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

// RootComplete checks the proof of one root.
func RootComplete(ctx context.Context, store StoreOps, ref *BlockRef) (bool, error) {
	complete, err := RootsComplete(ctx, store, []*BlockRef{ref})
	if err != nil {
		return false, err
	}
	return complete[0], nil
}

// RootsComplete checks the proofs of a batch of roots in one transaction. It
// reports the proof of each root in order; an empty root has none, and a store
// that keeps no proofs proves nothing.
func RootsComplete(ctx context.Context, store StoreOps, refs []*BlockRef) ([]bool, error) {
	p := rootProofs(store)
	if len(refs) == 0 || p == nil {
		return make([]bool, len(refs)), nil
	}
	return p.RootsComplete(ctx, refs)
}
