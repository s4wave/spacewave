package provider_spacewave

import "github.com/aperturerobotics/util/refcount"

// GetEntityKeyStore returns the shared entity key store.
func (a *ProviderAccount) GetEntityKeyStore() *EntityKeyStore {
	return a.getEntityKeyStore()
}

// RetainEntityKeypairStepUp retains unlocked entity keypairs until the returned
// reference is released.
func (a *ProviderAccount) RetainEntityKeypairStepUp() *refcount.Ref[struct{}] {
	return a.entityKeypairStepUp.Retain()
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
