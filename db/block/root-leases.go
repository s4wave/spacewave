package block

import "context"

// LeasedRoots holds a store's leased roots: a reader's pin on a root it reads,
// and a stage holding what a writer outside any named root builds. Each is a
// named root whose lease the store renews while the holding process lives, so
// a process that dies releases its pins and stages by silence. The named roots
// themselves are RootOps, and completion proofs are RootProofs.
type LeasedRoots interface {
	// HoldsRoots reports whether the store holds roots. The other methods
	// require it.
	HoldsRoots() bool
	// PinRoot holds a root until the returned release is called.
	PinRoot(context.Context, *BlockRef) (func(), error)
	// OpenStage returns a store whose writes a new stage holds until release.
	OpenStage(context.Context) (StoreOps, func(), error)
	// ReleaseRoots drops the stage's holds on roots, on a store OpenStage
	// returned.
	ReleaseRoots(context.Context, []*BlockRef) error
}

// HoldsRoots reports whether a store holds roots, pins and stages. A store
// that holds none never frees a block by liveness.
func HoldsRoots(store StoreOps) bool {
	r, ok := store.(LeasedRoots)
	return ok && r.HoldsRoots()
}

// PinRoot holds a root until release. The caller must already hold a named
// root, pin or stage that reaches the root while it takes the pin. Only the
// local provider's PinRoot fetches a block it lacks; the others pin the ref
// as it is.
func PinRoot(ctx context.Context, store StoreOps, ref *BlockRef) (func(), error) {
	if ref.GetEmpty() || !HoldsRoots(store) {
		return func() {}, nil
	}
	return store.(LeasedRoots).PinRoot(ctx, ref)
}

// OpenStage returns a store whose writes the stage holds, and the stage's
// release. The stage holds each block until a parent written outside the stage
// references it; after the release, the collector frees every block no other
// holder keeps. Hold the stage until the transaction that references the
// stage's roots returns. A store that holds no roots returns itself and a
// no-op release.
func OpenStage(ctx context.Context, store StoreOps) (StoreOps, func(), error) {
	if !HoldsRoots(store) {
		return store, func() {}, nil
	}
	return store.(LeasedRoots).OpenStage(ctx)
}

// ReleaseRoots drops the stage's holds on roots the caller wrote through a
// store OpenStage returned. Each root then survives only through another
// block's reference, a named root, or a pin.
func ReleaseRoots(ctx context.Context, store StoreOps, refs []*BlockRef) error {
	if len(refs) == 0 || !HoldsRoots(store) {
		return nil
	}
	return store.(LeasedRoots).ReleaseRoots(ctx, refs)
}
