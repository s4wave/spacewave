package world_expect

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ErrRootMoved is returned when an object no longer holds its expected root.
var ErrRootMoved = errors.New("object root moved")

// ExpectCurrent applies an ExpectRootsOp in the write transaction ws that
// expects each object to keep the root ws holds for it now. Call it after
// reading the objects a change is decided from and before writing any of
// them. Applied where ws was read, it always holds; a replay of the
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
		op.Roots = append(op.Roots, &ExpectedRoot{ObjectKey: key, RootRef: root})
	}

	// Record the expectation in the transaction.
	_, _, err := ws.ApplyWorldOp(ctx, op, "")
	return err
}

// ReadRoot returns the root block of an object, or nil when it does not
// exist. It reads no block.
func ReadRoot(ctx context.Context, ws world.WorldState, key string) (*block.BlockRef, error) {
	// Look the object up and read its root reference.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		return nil, err
	}
	ref, _, err := obj.GetRootRef(ctx)
	return ref.GetRootRef(), err
}

// SameRoot reports whether two roots are equal, where nil and empty roots
// mean the object does not exist.
func SameRoot(a, b *block.BlockRef) bool {
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
		if !SameRoot(got, want.GetRootRef()) {
			return false, errors.Wrap(ErrRootMoved, want.GetObjectKey())
		}
	}
	return false, nil
}

// _ is a type assertion.
var _ world.Operation = (*ExpectRootsOp)(nil)
