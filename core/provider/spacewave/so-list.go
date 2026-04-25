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
	allowed := hasSOListAccess(subStatus)

	var invalidate func()
	var changed bool
	a.soListBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
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
	if !allowed {
		a.soListCtr.SetValue(&sobject.SharedObjectList{})
		a.refreshSelfEnrollmentSummary(context.Background())
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

// invalidateSharedObjectList restarts the SO list owner when a full snapshot is stale.
func (a *ProviderAccount) invalidateSharedObjectList() {
	var invalidate func()
	a.soListBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		invalidate = a.soListInvalidate
	})
	if invalidate != nil {
		invalidate()
	}
}

// resolveSharedObjectList resolves the shared object list once for current subscribers.
func (a *ProviderAccount) resolveSharedObjectList(
	ctx context.Context,
	released func(),
) (struct{}, func(), error) {
	a.setSharedObjectListInvalidator(released)

	if !a.hasSharedObjectListAccess() {
		a.soListCtr.SetValue(&sobject.SharedObjectList{})
		a.refreshSelfEnrollmentSummary(ctx)
		return struct{}{}, nil, nil
	}
	if err := a.fetchSharedObjectList(ctx); err != nil {
		return struct{}{}, nil, err
	}
	return struct{}{}, nil, nil
}

// setSharedObjectListInvalidator stores the active invalidation callback.
func (a *ProviderAccount) setSharedObjectListInvalidator(invalidate func()) {
	a.soListBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.soListInvalidate = invalidate
		broadcast()
	})
}

// EnsureSharedObjectListLoaded resolves the SO list owner and waits for the current snapshot.
func (a *ProviderAccount) EnsureSharedObjectListLoaded(ctx context.Context) error {
	_, rel, err := a.soListRc.Resolve(ctx)
	if err != nil {
		return err
	}
	defer rel()

	_, err = a.soListCtr.WaitValue(ctx, nil)
	return err
}
