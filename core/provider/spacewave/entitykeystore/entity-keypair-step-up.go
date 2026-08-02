package entitykeystore

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// EntityKeypairStepUp retains unlocked entity keypairs for step-up consumers.
type EntityKeypairStepUp struct {
	store func() *EntityKeyStore
	rc    *refcount.RefCount[struct{}]
}

// NewEntityKeypairStepUp constructs an EntityKeypairStepUp reading from store.
func NewEntityKeypairStepUp(ctx context.Context, store func() *EntityKeyStore) *EntityKeypairStepUp {
	stepUp := &EntityKeypairStepUp{
		store: store,
	}
	stepUp.rc = refcount.NewRefCount(ctx, false, nil, nil, stepUp.resolve)
	return stepUp
}

// Retain retains unlocked entity keypairs until the returned reference is released.
func (s *EntityKeypairStepUp) Retain() *refcount.Ref[struct{}] {
	return s.rc.AddRef(nil)
}

// Resolve retains unlocked entity keypairs and waits for the retention to resolve.
func (s *EntityKeypairStepUp) Resolve(ctx context.Context) (struct{}, func(), error) {
	return s.rc.Resolve(ctx)
}

func (s *EntityKeypairStepUp) resolve(_ context.Context, _ func()) (struct{}, func(), error) {
	store := s.store()
	if store == nil {
		return struct{}{}, nil, nil
	}
	ref := store.Retain()
	return struct{}{}, ref.Release, nil
}
