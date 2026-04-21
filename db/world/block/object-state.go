package world_block

import (
	"context"
	"runtime/trace"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// ObjectState implements the ObjectState interface attached to block cursor.
type ObjectState struct {
	w   *WorldState
	bcs *block.Cursor
	key string
}

// NewObjectState constructs a new ObjectState from a block cursor and world state.
func NewObjectState(ctx context.Context, w *WorldState, bcs *block.Cursor) (*ObjectState, error) {
	s := &ObjectState{w: w, bcs: bcs}
	obj, err := s.GetRoot(ctx)
	if err != nil {
		return nil, err
	}
	s.key = obj.GetKey()
	if s.key == "" {
		return nil, world.ErrEmptyObjectKey
	}
	return s, nil
}

// GetKey returns the key this state object is for.
func (o *ObjectState) GetKey() string {
	return o.key
}

// GetRootRef returns the root reference of the object.
func (o *ObjectState) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	root, err := o.GetRoot(ctx)
	if err != nil {
		return nil, 0, err
	}
	return root.GetRootRef(), root.GetRev(), nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (o *ObjectState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	var err error
	if ref.GetEmpty() {
		ref, _, err = o.GetRootRef(ctx)
		if err != nil {
			return err
		}
	}
	return o.w.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (o *ObjectState) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	if err := nref.Validate(); err != nil {
		return 0, err
	}
	root, err := o.GetRoot(ctx)
	if err != nil {
		return 0, err
	}
	if root.GetRootRef().EqualsRef(nref) {
		// no-op
		return root.GetRev(), nil
	}

	prevBlk := root.Clone()

	root = root.Clone()
	root.RootRef = nref
	root.Rev++
	r := root.Rev

	o.bcs.SetBlock(root, true)

	changeBcs, err := o.w.queueWorldChange(ctx, &WorldChange{
		Key:        o.key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return r, err
	}
	if changeBcs != nil {
		nbcs := o.bcs
		changeBcs.SetRef(6, nbcs)
		prevBcs := o.bcs.Detach(false) // clone bcs for previous revision
		prevBcs.SetBlock(prevBlk, true)
		changeBcs.SetRef(7, prevBcs)
	}

	// GC: swap object -> block edge (old -> new).
	if rg := o.w.refGraph; rg != nil {
		taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/object-state/set-root-ref/update-gc-refs")
		oldBlockRef := prevBlk.GetRootRef().GetRootRef()
		newBlockRef := nref.GetRootRef()
		objIRI := block_gc.ObjectIRI(o.key)
		var adds []block_gc.RefEdge
		var removes []block_gc.RefEdge
		if oldBlockRef != nil && !oldBlockRef.GetEmpty() {
			oldIRI := block_gc.BlockIRI(oldBlockRef)
			removes = append(removes, block_gc.RefEdge{Subject: objIRI, Object: oldIRI})

			queryCtx, queryTask := trace.NewTask(taskCtx, "hydra/world-block/object-state/set-root-ref/has-incoming-refs-excluding")
			has, err := rg.HasIncomingRefsExcluding(queryCtx, oldIRI, objIRI)
			queryTask.End()
			if err != nil {
				subtask.End()
				return r, err
			}
			if !has {
				adds = append(adds, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: oldIRI})
			}
		}
		if newBlockRef != nil && !newBlockRef.GetEmpty() {
			newIRI := block_gc.BlockIRI(newBlockRef)
			adds = append(adds, block_gc.RefEdge{Subject: objIRI, Object: newIRI})
			removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: newIRI})
		}
		applyCtx, applyTask := trace.NewTask(taskCtx, "hydra/world-block/object-state/set-root-ref/apply-ref-batch")
		err := rg.ApplyRefBatch(applyCtx, adds, removes)
		applyTask.End()
		subtask.End()
		if err != nil {
			return r, err
		}
	}

	return r, nil
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (o *ObjectState) ApplyObjectOp(
	rctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}
	if err := op.Validate(); err != nil {
		return 0, false, err
	}

	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()

	sysErr, err := op.ApplyWorldObjectOp(ctx, o.w.le, o, opSender)
	if err != nil {
		return 0, sysErr, err
	}

	_, rev, err := o.GetRootRef(ctx)
	if err != nil {
		return rev, true, err
	}

	return rev, false, nil
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (o *ObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	return o.incrementRev(ctx, true)
}

// incrementRev increments the object rev optionally adding a changelog entry.
func (o *ObjectState) incrementRev(ctx context.Context, addToChangelog bool) (uint64, error) {
	root, err := o.GetRoot(ctx)
	if err != nil {
		return 0, err
	}
	nrev := root.Rev + 1
	if addToChangelog {
		_, err = o.w.queueWorldChange(ctx, &WorldChange{
			Key:        o.key,
			ChangeType: WorldChangeType_WorldChange_OBJECT_INC_REV,
			ObjectRev:  nrev,
		})
		if err != nil {
			return 0, err
		}
	}
	root = root.Clone()
	root.Rev = nrev
	o.bcs.SetBlock(root, true)
	return nrev, nil
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (o *ObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		currSeqno, err := o.w.GetSeqno(ctx)
		if err != nil {
			return 0, err
		}

		_, currRev, err := o.GetRootRef(ctx)
		if err != nil {
			if err == world.ErrObjectNotFound && ignoreNotFound {
				_, err = o.w.WaitSeqno(ctx, currSeqno+1)
				if err != nil {
					return 0, err
				}
				continue
			}
			return 0, err
		}

		if currRev >= rev {
			return currRev, nil
		}

		_, err = o.w.WaitSeqno(ctx, currSeqno+1)
		if err != nil {
			return 0, err
		}
	}
}

// GetRoot unmarshals root from the block cursor
func (o *ObjectState) GetRoot(ctx context.Context) (*Object, error) {
	return UnmarshalObject(ctx, o.bcs)
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
