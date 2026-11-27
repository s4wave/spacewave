package provider_spacewave

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// syncSharedObjectListAccess updates whether the SO list endpoint is permitted.
func (a *ProviderAccount) syncSharedObjectListAccess(
	subStatus s4wave_provider_spacewave.BillingStatus,
) {
	// Record the access flag and take the invalidator when it changed.
	allowed := hasSOListAccess(subStatus)
	var invalidate func() bool
	var changed bool
	a.soListBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Store the flag and take the invalidator only on a change.
		if a.soListAccess == allowed {
			return
		}
		a.soListAccess = allowed
		invalidate = a.soListInvalidate
		changed = true
		broadcast()
	})
	if !changed {
		return
	}

	// Clear the list when access is revoked and restart its resolution.
	if !allowed {
		a.invalidateSharedObjectLookup()
		a.soListCtr.SetValue(&sobject.SharedObjectList{})
		a.selfEnrollmentRefresh.Trigger()
	}
	if invalidate != nil {
		invalidate()
	}
}

// hasSharedObjectListAccess returns true when the cached subscription state permits fetching the SO list.
func (a *ProviderAccount) hasSharedObjectListAccess() bool {
	var allowed bool
	a.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		allowed = a.soListAccess
	})
	return allowed
}

// invalidateSharedObjectList restarts soListRc when a full snapshot is stale.
// It returns whether a refetch started.
func (a *ProviderAccount) invalidateSharedObjectList() bool {
	var invalidate func() bool
	a.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		invalidate = a.soListInvalidate
	})
	return invalidate != nil && invalidate()
}

// resolveSharedObjectList resolves the shared object list once for current subscribers.
func (a *ProviderAccount) resolveSharedObjectList(
	ctx context.Context,
	_ func(),
) (struct{}, func(), error) {
	a.setSharedObjectListInvalidator(a.soListRc.Invalidate)

	// Without list access no revisions arrive to reconcile metadata against.
	if !a.hasSharedObjectListAccess() {
		a.soListCtr.SetValue(&sobject.SharedObjectList{})
		a.selfEnrollmentRefresh.Trigger()
		a.InvalidateSharedObjectMetadataCache()
		return struct{}{}, nil, nil
	}
	if err := a.fetchSharedObjectList(ctx); err != nil {
		return struct{}{}, nil, err
	}
	return struct{}{}, nil, nil
}

// setSharedObjectListInvalidator stores the active invalidation callback.
func (a *ProviderAccount) setSharedObjectListInvalidator(invalidate func() bool) {
	a.soListBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.soListInvalidate = invalidate
		broadcast()
	})
}

// EnsureSharedObjectListLoaded resolves soListRc and waits for the current snapshot.
func (a *ProviderAccount) EnsureSharedObjectListLoaded(ctx context.Context) error {
	// Hold a reference so the list resolves.
	_, rel, err := a.soListRc.Resolve(ctx)
	if err != nil {
		return err
	}
	defer rel()

	// Wait for the resolved snapshot.
	_, err = a.soListCtr.WaitValue(ctx, nil)
	return err
}
