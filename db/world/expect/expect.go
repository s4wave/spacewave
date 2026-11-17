// Package world_expect provides world/expect-roots, a World operation that
// makes a write transaction conditional on objects it read keeping their
// roots. The condition is recorded in the transaction, so it holds where the
// transaction was decided and is checked again on every Device that replays
// it, including for objects the transaction reads but does not write.
//
// Replay waits at an operation whose type no handler resolves, rather than
// refusing it, so every Device decides each position alike. A Device on a
// build without this operation therefore stops applying a Space's operations
// at the first transaction that records it, until it upgrades: every Device
// of a Space rolls forward together.
package world_expect

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ErrRootMoved is returned when an object no longer holds its expected root.
var ErrRootMoved = errors.New("object root moved")

// ExpectCurrent applies an ExpectRootsOp in the write transaction ws that
// expects each object to keep the root ws holds for it now. Call it after
// reading the objects a change is decided from and before writing any of
// them, naming every mutable object the decision read, whether or not the
// transaction writes it; an object left out can move under a replay. Applied where ws was read, it always holds; a replay of the
// transaction onto a World where another writer moved one of the objects
// fails it, which refuses the transaction.
func ExpectCurrent(ctx context.Context, ws world.WorldState, keys ...string) error {
	// Read the root of each object.
	op := &ExpectRootsOp{Roots: make([]*ExpectedRoot, 0, len(keys))}
	for _, key := range keys {
		root, err := ReadRoot(ctx, ws, key)
		if err != nil {
			return err
		}
		op.Roots = append(op.Roots, root)
	}

	// Record the expectation in the transaction.
	_, _, err := ws.ApplyWorldOp(ctx, op, "")
	return err
}

// ReadRoot returns the root an object holds now, which is Absent when it does
// not exist. It reads no block.
func ReadRoot(ctx context.Context, ws world.WorldState, key string) (*ExpectedRoot, error) {
	// Look the object up and read its root reference.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !found {
		return &ExpectedRoot{ObjectKey: key, Absent: true}, nil
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	return &ExpectedRoot{ObjectKey: key, RootRef: ref.GetRootRef()}, nil
}

// Matches reports whether two roots of one object are the same: both absent,
// or both present with equal root blocks, where an empty root equals only an
// empty root.
func (r *ExpectedRoot) Matches(other *ExpectedRoot) bool {
	if r.GetAbsent() || other.GetAbsent() {
		return r.GetAbsent() == other.GetAbsent()
	}
	a, b := r.GetRootRef(), other.GetRootRef()
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}

// Validate requires a key for each expected root.
func (o *ExpectRootsOp) Validate() error {
	for i, root := range o.GetRoots() {
		if root.GetObjectKey() == "" {
			return errors.Errorf("expect roots: root %d has no object key", i)
		}
	}
	return nil
}

// ApplyWorldOp fails with ErrRootMoved unless each object holds its expected
// root, and changes nothing.
func (o *ExpectRootsOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	for _, want := range o.GetRoots() {
		got, err := ReadRoot(ctx, ws, want.GetObjectKey())
		if err != nil {
			return false, err
		}
		if !got.Matches(want) {
			return false, errors.Wrap(ErrRootMoved, want.GetObjectKey())
		}
	}
	return false, nil
}

// _ is a type assertion.
var _ world.Operation = (*ExpectRootsOp)(nil)
