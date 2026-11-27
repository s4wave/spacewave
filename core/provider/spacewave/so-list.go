package provider_spacewave

import (
	"context"
	"sync/atomic"

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

// createdSharedObjects orders the Spaces this account created against the
// catalog snapshots fetched around them, so a snapshot that omits a created
// Space drops it once the snapshot was taken after the create.
//
// The cloud commits a Space before CreateSharedObject returns, so a list
// request sent after that return reflects the Space. A create takes its number
// after the cloud's reply and a fetch reads the counter before it sends, so a
// snapshot whose number is at least the create's was taken after the create,
// and its omission means the account no longer holds the Space.
type createdSharedObjects struct {
	// seq is the number the last create took.
	seq atomic.Uint64
	// entries maps the ID of each entry the catalog holds as created to the
	// number its create took. Only soListCtr.SwapValue callbacks touch it,
	// under the container's lock.
	entries map[string]uint64
}

// add records seq as the number soID's create took.
// The caller holds soListCtr's lock.
func (c *createdSharedObjects) add(soID string, seq uint64) {
	if c.entries == nil {
		c.entries = make(map[string]uint64)
	}
	c.entries[soID] = seq
}

// merge returns snapshot with each created entry of cached that snapshot
// omits and may predate: one created after the fetch that took snapshot read
// since. It forgets every other entry, which the snapshot now decides. The
// caller holds soListCtr's lock.
func (c *createdSharedObjects) merge(
	snapshot *sobject.SharedObjectList,
	cached *sobject.SharedObjectList,
	since uint64,
) *sobject.SharedObjectList {
	// Copy the snapshot and index its IDs.
	next := snapshot.CloneVT()
	if next == nil {
		next = &sobject.SharedObjectList{}
	}
	seen := make(map[string]bool, len(next.GetSharedObjects()))
	for _, entry := range next.GetSharedObjects() {
		if soID := entry.GetRef().GetProviderResourceRef().GetId(); soID != "" {
			seen[soID] = true
		}
	}

	// Keep each created entry newer than the snapshot, and forget the rest.
	kept := make(map[string]uint64)
	for _, entry := range cached.GetSharedObjects() {
		soID := entry.GetRef().GetProviderResourceRef().GetId()
		seq, ok := c.entries[soID]
		if !ok || entry.GetSource() != "created" || seen[soID] || seq <= since {
			continue
		}
		next.SharedObjects = append(next.SharedObjects, entry.CloneVT())
		kept[soID] = seq
		seen[soID] = true
	}
	c.entries = kept
	return next
}
