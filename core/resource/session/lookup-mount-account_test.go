package resource_session_test

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
)

// lookupMountAccount wraps a real local account with a gated cloud catalog seam.
type lookupMountAccount struct {
	// ProviderAccount retains the real account's remaining features.
	provider.ProviderAccount
	// SharedObjectProvider retains real local create, delete and mount operations.
	sobject.SharedObjectProvider
	// list is held at loading or at a snapshot missing the requested Space.
	list *ccontainer.CContainer[*sobject.SharedObjectList]
	// lookup performs the signed single-ID request.
	lookup func(context.Context, string) (*sobject.SharedObjectListEntry, error)
	// lookups counts targeted calls made by the mount path.
	lookups int
	// refreshes counts explicit full-catalog reloads.
	refreshes int
}

// GetProviderAccountFeature preserves targeted lookup through a feature wrapper.
func (a *lookupMountAccount) GetProviderAccountFeature(ctx context.Context, feature provider.ProviderFeature) (provider.ProviderAccountFeature, error) {
	if feature == provider.ProviderFeature_ProviderFeature_SHARED_OBJECT {
		return a, nil
	}
	return a.ProviderAccount.GetProviderAccountFeature(ctx, feature)
}

// AccessSharedObjectList exposes the catalog without releasing its loading gate.
func (a *lookupMountAccount) AccessSharedObjectList(context.Context, func()) (ccontainer.Watchable[*sobject.SharedObjectList], func(), error) {
	return a.list, func() {}, nil
}

// LookupSharedObject supplies the real local Space's descriptor on one miss.
func (a *lookupMountAccount) LookupSharedObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	a.lookups++
	return a.lookup(ctx, id)
}

// RefreshSharedObjectList records unwanted full-catalog reloads.
func (a *lookupMountAccount) RefreshSharedObjectList(context.Context) error {
	a.refreshes++
	return nil
}

var _ sobject.SharedObjectProvider = (*lookupMountAccount)(nil)
var _ provider.ProviderAccount = (*lookupMountAccount)(nil)
