package world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// AssertObjectRev asserts that an object is at a given rev.
func AssertObjectRev(obj ObjectState, expected uint64) error {
	_, rev, err := obj.GetRootRef()
	if err == nil && rev != expected {
		err = errors.Wrapf(ErrUnexpectedRev, "expected %d got %d", expected, rev)
	}
	return err
}

// LookupRootRef gets an object and returns its root reference and rev.
//
// If not found, returns nil, 0, nil.
func LookupRootRef(eng Engine, key string) (*bucket.ObjectRef, uint64, error) {
	stx, err := eng.NewTransaction(false)
	if err != nil {
		return nil, 0, err
	}
	defer stx.Discard()

	obj, found, err := stx.GetObject(key)
	if err != nil {
		return nil, 0, err
	}

	if !found {
		return nil, 0, nil
	}
	return obj.GetRootRef()
}

// ApplyWaitObjectOp applies an ObjectOp and waits for it to be confirmed.
// Returns the updated revision.
func ApplyWaitObjectOp(
	ctx context.Context,
	obj ObjectState,
	op Operation,
	opSender peer.ID,
) (rev uint64, sysErr bool, err error) {
	rev, sysErr, err = obj.ApplyObjectOp(op, opSender)
	if err != nil {
		return
	}
	nrev, err := obj.WaitRev(ctx, rev, false)
	if err != nil {
		return rev, true, err
	}
	return nrev, false, nil
}

// LookupObject looks up & unmarshals an object from the world.
func LookupObject[T block.Block](
	ctx context.Context,
	ws WorldState,
	objKey string,
	ctor func() block.Block,
) (out T, err error) {
	obj, err := MustGetObject(ws, objKey)
	if err != nil {
		return out, err
	}
	_, _, err = AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		out, err = block.UnmarshalBlock[T](bcs, ctor)
		return err
	})
	return out, err
}

// LookupObjectState looks up & unmarshals an object ref from the world.
func LookupObjectState[T block.Block](
	ctx context.Context,
	access AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	ctor func() block.Block,
) (out T, err error) {
	_, err = AccessObject(ctx, access, ref, func(bcs *block.Cursor) error {
		var err error
		out, err = block.UnmarshalBlock[T](bcs, ctor)
		return err
	})
	return out, err
}
