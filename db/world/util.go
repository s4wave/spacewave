package world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// ExecTransaction executes a transaction inside a function callback.
//
// If a write transaction, calls Commit if the callback return nil.
// Otherwise, discards the transaction.
func ExecTransaction(
	ctx context.Context,
	eng Engine,
	write bool,
	cb func(ctx context.Context, wtx WorldState) error,
) error {
	wtx, err := eng.NewTransaction(ctx, write)
	if err != nil {
		return err
	}
	defer wtx.Discard()

	if err := cb(ctx, wtx); err != nil {
		return err
	}

	if !write {
		return nil
	}

	return wtx.Commit(ctx)
}

// AssertObjectRev asserts that an object is at a given rev.
func AssertObjectRev(ctx context.Context, obj ObjectState, expected uint64) error {
	_, rev, err := obj.GetRootRef(ctx)
	if err == nil && rev != expected {
		err = errors.Wrapf(ErrUnexpectedRev, "expected %d got %d", expected, rev)
	}
	return err
}

// LookupRootRef gets an object and returns its root reference and rev.
//
// If not found, returns nil, 0, nil.
func LookupRootRef(ctx context.Context, eng Engine, key string) (*bucket.ObjectRef, uint64, error) {
	stx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer stx.Discard()

	obj, found, err := stx.GetObject(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	if !found {
		return nil, 0, nil
	}
	return obj.GetRootRef(ctx)
}

// LookupObject looks up & unmarshals an object from the world.
func LookupObject[T block.Block](
	ctx context.Context,
	ws WorldState,
	objKey string,
	ctor func() block.Block,
) (out T, objRef ObjectState, err error) {
	obj, err := MustGetObject(ctx, ws, objKey)
	if err != nil {
		return out, nil, err
	}
	_, _, err = AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		out, err = block.UnmarshalBlock[T](ctx, bcs, ctor)
		return err
	})
	return out, obj, err
}

// LookupObjectRef looks up & unmarshals an object ref from the world.
func LookupObjectRef[T block.Block](
	ctx context.Context,
	access AccessWorldStateFunc,
	ref *bucket.ObjectRef,
	ctor func() block.Block,
) (out T, err error) {
	_, err = AccessObject(ctx, access, ref, func(bcs *block.Cursor) error {
		var err error
		out, err = block.UnmarshalBlock[T](ctx, bcs, ctor)
		return err
	})
	return out, err
}

// CollectObjectBodies looks up and unmarshals the objects with the given keys.
//
// ctor must return an object of type T
// returns two slices of length objKeys
// if any objects are not found, returns nil for that object state / value and objs, objsStates, ErrNotFound
// returns nil, nil, err for any other error
func CollectObjectBodies[T block.Block](
	ctx context.Context,
	ws WorldState,
	objKeys []string,
	ctor func() block.Block,
) ([]T, []ObjectState, error) {
	objs := make([]T, len(objKeys))
	objStates := make([]ObjectState, len(objKeys))
	var retErr error
	for i, objKey := range objKeys {
		obj, objState, err := LookupObject[T](ctx, ws, objKey, ctor)
		if err != nil {
			if err == ErrObjectNotFound {
				retErr = err
				continue
			}
			return nil, nil, err
		}
		objs[i] = obj
		objStates[i] = objState
	}

	return objs, objStates, retErr
}
