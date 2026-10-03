package sobject_world_engine

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// maxRetainedRoots is the most retained roots a World holds. The roots live in
// the SharedObject state, which has a small size limit.
const maxRetainedRoots = 16

// maxRetainedRootNameLen is the longest retained root name in bytes.
const maxRetainedRootNameLen = 64

// retainedRootsName names the local root that holds the retained root set.
const retainedRootsName = "retained-roots"

// retainedRootsProofStoreID is the local state store holding the completion
// proofs of the retained roots.
const retainedRootsProofStoreID = "retained-root-retention"

// errRetainedRootNotHead is returned when the root to retain is no longer the
// installed World head.
var errRetainedRootNotHead = errors.New("root is not the current World head, export it again")

// SetRetainedRoot adds a SetRetainedRootOp to the operation set and returns
// its replay outcome. ref must be the installed World head or empty.
func (e *soEngine) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	// Reject a malformed name.
	if err := validateRetainedRootName(name); err != nil {
		return err
	}

	// Build the operation on the current World, excluding write transactions
	// until it is placed.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	if _, err := e.advance(ctx, snap); err != nil {
		return err
	}
	_, head, _ := e.replay.head()
	if !ref.GetEmpty() && !ref.EqualVT(head.GetHeadRef().GetRootRef()) {
		return errRetainedRootNotHead
	}

	// Add it and wait for its outcome.
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_SetRetainedRoot{
			SetRetainedRoot: &SetRetainedRootOp{Name: name, RootRef: ref},
		},
	}).MarshalVT()
	if err != nil {
		return err
	}
	return e.queueOperation(ctx, opData, nil)
}

// processSetRetainedRootOp sets or releases one retained root.
func processSetRetainedRootOp(
	le *logrus.Entry,
	setOp *SetRetainedRootOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Check the name.
	if err := validateRetainedRootName(setOp.GetName()); err != nil {
		return rejectOp(le, peerID, nonce, err.Error())
	}

	// Replace, add, or remove the entry, keeping the list sorted by name.
	roots := slices.Clone(headState.GetRetainedRoots())
	idx, found := slices.BinarySearchFunc(roots, setOp.GetName(), func(root *RetainedRoot, name string) int {
		return strings.Compare(root.GetName(), name)
	})
	switch {
	case setOp.GetRootRef().GetEmpty() && found:
		roots = slices.Delete(roots, idx, idx+1)
	case setOp.GetRootRef().GetEmpty():
	case found:
		roots[idx] = &RetainedRoot{Name: setOp.GetName(), RootRef: setOp.GetRootRef()}
	case len(roots) >= maxRetainedRoots:
		return rejectOp(le, peerID, nonce, "too many retained roots")
	default:
		roots = slices.Insert(roots, idx, &RetainedRoot{Name: setOp.GetName(), RootRef: setOp.GetRootRef()})
	}

	// Accept it.
	nextHeadState := headState.CloneVT()
	nextHeadState.RetainedRoots = roots
	return nextHeadState, sobject.BuildSOOperationResult(peerID.String(), nonce, true, nil), nil
}

// validateRetainedRootName checks a retained root name.
func validateRetainedRootName(name string) error {
	if name == "" || len(name) > maxRetainedRootNameLen {
		return errors.Errorf("retained root name must be 1 to %d bytes", maxRetainedRootNameLen)
	}
	return nil
}

// retainRoots copies every retained root's World graph into the local store
// and holds the set under one local named root, releasing roots no longer in
// the set. Returns block.ErrNotFound when a root's block is missing locally
// and from storage. Callers serialize calls through the writer lock.
func (c *Controller) retainRoots(ctx context.Context, so sobject.SharedObject, roots []*RetainedRoot) error {
	// Release the set when it is empty.
	store := so.GetBlockStore()
	if len(roots) == 0 {
		return block.SetRetainedRoot(ctx, store, retainedRootsName, nil)
	}

	// Copy each root's graph.
	proofs, release, err := so.AccessLocalStateStore(ctx, retainedRootsProofStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	refs := make([]*block.BlockRef, 0, len(roots))
	for _, root := range roots {
		head := &bucket.ObjectRef{BucketId: store.GetID(), RootRef: root.GetRootRef()}
		if err := RetainWorld(ctx, so, head, proofs, nil); err != nil {
			return errors.Wrapf(err, "retained root %q", root.GetName())
		}
		refs = append(refs, root.GetRootRef())
	}

	// Write the set block referencing the roots and retain it.
	data, err := (&RetainedRootSet{Roots: roots}).MarshalVT()
	if err != nil {
		return err
	}
	setRef, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return err
	}
	entry := &block.PutBatchEntry{Ref: setRef, Data: data, Refs: refs}
	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		return err
	}
	return block.SetRetainedRoot(ctx, store, retainedRootsName, setRef)
}

// _ is a type assertion
var _ world.RootRetainingEngine = (*soEngine)(nil)
