package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// GetEntityKeyStore returns the shared entity key store.
func (a *ProviderAccount) GetEntityKeyStore() *EntityKeyStore {
	return a.getEntityKeyStore()
}

// GetEntityKeypairTracker returns the shared entity key store.
func (a *ProviderAccount) GetEntityKeypairTracker() *EntityKeyStore {
	return a.getEntityKeyStore()
}

// RetainEntityKeypairStepUp retains unlocked entity keypairs until the returned
// reference is released.
func (a *ProviderAccount) RetainEntityKeypairStepUp() *refcount.Ref[struct{}] {
	return a.entityKeypairStepUpRc.AddRef(nil)
}

// resolveEntityKeypairStepUp holds a store retention ref while step-up
// consumers are mounted.
func (a *ProviderAccount) resolveEntityKeypairStepUp(
	_ context.Context,
	_ func(),
) (struct{}, func(), error) {
	store := a.getEntityKeyStore()
	if store == nil {
		return struct{}{}, nil, nil
	}
	ref := store.Retain()
	return struct{}{}, ref.Release, nil
}

func (a *ProviderAccount) getEntityKeyStore() *EntityKeyStore {
	if a.entityKeyStore != nil {
		return a.entityKeyStore
	}
	if a.p == nil {
		return nil
	}
	a.entityKeyStore = a.p.GetEntityKeyStore(a.accountID)
	return a.entityKeyStore
}
