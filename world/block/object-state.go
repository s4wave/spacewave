package world_block

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
)

// ObjectState implements the ObjectState interface attached to block cursor.
type ObjectState struct {
	w   *WorldState
	bcs *block.Cursor
	key string
}

// NewObjectState constructs a new ObjectState from a block cursor and world state.
func NewObjectState(w *WorldState, bcs *block.Cursor) (*ObjectState, error) {
	s := &ObjectState{w: w, bcs: bcs}
	obj, err := s.getRoot()
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
func (o *ObjectState) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	root, err := o.getRoot()
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
		ref, _, err = o.GetRootRef()
		if err != nil {
			return err
		}
	}
	return o.w.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (o *ObjectState) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	if err := nref.Validate(); err != nil {
		return 0, err
	}
	root, err := o.getRoot()
	if err != nil {
		return 0, err
	}
	if root.GetRootRef().EqualsRef(nref) {
		// no-op
		return root.GetRev(), nil
	}
	prevBlk := root.Clone()
	prevBcs := o.bcs.Detach(false) // clone bcs for previous revision
	prevBcs.SetBlock(prevBlk, true)
	root.RootRef = nref
	root.Rev++
	r := root.Rev
	o.bcs.SetBlock(root, true)
	changeBcs, err := o.w.queueWorldChange(&WorldChange{
		Key:        o.key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return r, err
	}
	if changeBcs != nil {
		nbcs := o.bcs
		changeBcs.SetRef(6, nbcs)
		changeBcs.SetRef(7, prevBcs)
	}
	return r, nil
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (o *ObjectState) ApplyObjectOp(
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}

	subCtx, subCtxCancel := context.WithCancel(o.w.ctx)
	defer subCtxCancel()

	sysErr, err := op.ApplyWorldObjectOp(subCtx, o, opSender)
	if err != nil {
		return 0, sysErr, err
	}

	_, rev, err := o.GetRootRef()
	if err != nil {
		return rev, true, err
	}
	return rev, false, nil
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (o *ObjectState) IncrementRev() (uint64, error) {
	return o.incrementRev(true)
}

// incrementRev increments the object rev optionally adding a changelog entry.
func (o *ObjectState) incrementRev(addToChangelog bool) (uint64, error) {
	root, err := o.getRoot()
	if err != nil {
		return 0, err
	}
	nrev := root.Rev + 1
	if addToChangelog {
		_, err = o.w.queueWorldChange(&WorldChange{
			Key:        o.key,
			ChangeType: WorldChangeType_WorldChange_OBJECT_INC_REV,
			ObjectRev:  nrev,
		})
		if err != nil {
			return 0, err
		}
	}
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
	// TODO this will likely be: wait for a local writer to increment rev
	// i.e. it will wait for someone else to change the block graph
	return 0, errors.New("TODO world/block object-state wait rev")
}

// getRoot unmarshals root from the block cursor
func (o *ObjectState) getRoot() (*Object, error) {
	obji, err := o.bcs.Unmarshal(NewObjectBlock)
	if err != nil {
		return nil, err
	}
	if obji == nil {
		return nil, world.ErrObjectNotFound
	}
	v, ok := obji.(*Object)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return v, nil
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
