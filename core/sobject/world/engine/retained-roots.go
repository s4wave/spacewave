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

// setRetainedRootAttempts bounds the resubmissions of a SetRetainedRootOp
// rejected because a storage reclaim pass advanced the generation.
const setRetainedRootAttempts = 3

// errRetainedRootNotHead is returned when the root to retain is no longer the
// accepted World head.
var errRetainedRootNotHead = errors.New("root is not the accepted World head, export it again")

// SetRetainedRoot submits a SetRetainedRootOp and waits for the validator's
// decision. ref must be the accepted head, so the operation names the
// generation the head was accepted on: until that generation advances, no
// reclaim pass has judged the head's blocks. A rejection caused by a
// concurrent advance resubmits while ref is still the head, which the
// validator holds through the pass.
func (e *soEngine) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	if err := validateRetainedRootName(name); err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		var generation uint64
		rejected, err := e.c.commitMaintenanceOp(ctx, e.so, func(state *InnerState) (*SOWorldOp, error) {
			if !ref.GetEmpty() && !ref.EqualVT(state.GetHeadRef().GetRootRef()) {
				return nil, errRetainedRootNotHead
			}
			generation = state.GetStorageGeneration()
			return &SOWorldOp{
				Body: &SOWorldOp_SetRetainedRoot{
					SetRetainedRoot: &SetRetainedRootOp{
						Name:              name,
						RootRef:           ref,
						StorageGeneration: generation,
					},
				},
			}, nil
		})
		if !rejected || attempt == setRetainedRootAttempts {
			return err
		}

		// Resubmit only when the generation moved past the operation.
		snap, serr := e.so.GetSharedObjectState(ctx)
		if serr != nil {
			return serr
		}
		state, serr := ReadInnerState(ctx, snap)
		if serr != nil {
			return serr
		}
		if state.GetStorageGeneration() == generation {
			return err
		}
	}
}

// processSetRetainedRootOp sets or releases one retained root. The operation
// must name the accepted storage generation: after an advance, the reclaim
// pass may be deleting packs that hold blocks the validator has not copied.
func processSetRetainedRootOp(
	le *logrus.Entry,
	setOp *SetRetainedRootOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Check the name and the generation.
	if err := validateRetainedRootName(setOp.GetName()); err != nil {
		return rejectOp(le, peerID, nonce, err.Error())
	}
	if setOp.GetStorageGeneration() != headState.GetStorageGeneration() {
		return rejectOp(le, peerID, nonce, "storage generation is stale")
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
// the set. The local store is the liveness test of storage reclaim, so the
// participant running reclaim calls this before each pass. Returns
// block.ErrNotFound when a root's block is missing locally and from storage.
func (c *Controller) retainRoots(ctx context.Context, so sobject.SharedObject, roots []*RetainedRoot) error {
	// Serialize with other retainRoots calls.
	c.retainMtx.Lock()
	defer c.retainMtx.Unlock()

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
