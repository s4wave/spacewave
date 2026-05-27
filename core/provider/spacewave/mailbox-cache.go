package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/mailboxcache"
	"github.com/s4wave/spacewave/core/provider/spacewave/seedflight"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// getPendingMailboxCache returns the pending mailbox cache.
func (a *ProviderAccount) getPendingMailboxCache() *mailboxcache.Store {
	var store *mailboxcache.Store
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.pendingMailboxCache == nil {
			a.state.pendingMailboxCache = mailboxcache.NewStore()
		}
		store = a.state.pendingMailboxCache
	})
	return store
}

// getPendingMailboxSeedLocked returns the singleflight seed for an SO.
func (a *ProviderAccount) getPendingMailboxSeedLocked(soID string) *seedflight.Seed {
	if a.state.pendingMailboxSeeds == nil {
		a.state.pendingMailboxSeeds = make(map[string]*seedflight.Seed)
	}
	seed := a.state.pendingMailboxSeeds[soID]
	if seed == nil {
		seed = &seedflight.Seed{}
		a.state.pendingMailboxSeeds[soID] = seed
	}
	return seed
}

// GetPendingMailboxEntriesSnapshot returns the cached pending mailbox entries for an SO.
func (a *ProviderAccount) GetPendingMailboxEntriesSnapshot(
	soID string,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, bool) {
	return a.getPendingMailboxCache().EntriesSnapshot(soID)
}

// GetPendingMailboxEntriesCached returns the cached pending mailbox entries,
// seeding once via HTTP if the cache is not yet valid. Concurrent callers
// share a single in-flight seed request via seedflight.Seed; subsequent calls
// return the cached snapshot and updates arrive via ApplyMailboxEntryEvent.
func (a *ProviderAccount) GetPendingMailboxEntriesCached(
	ctx context.Context,
	soID string,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, error) {
	if !a.canAccessOwnerMailbox() {
		a.setPendingMailboxResponse(soID, &api.GetMailboxResponse{})
		return nil, nil
	}
	if entries, valid := a.GetPendingMailboxEntriesSnapshot(soID); valid {
		return entries, nil
	}

	var seed *seedflight.Seed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = a.getPendingMailboxSeedLocked(soID)
	})

	if err := seed.Run(ctx, &a.accountBcast, func(ctx context.Context) error {
		return a.syncPendingMailboxEntries(ctx, soID)
	}); err != nil {
		return nil, err
	}

	entries, _ := a.GetPendingMailboxEntriesSnapshot(soID)
	return entries, nil
}

// getPendingMailboxResponseSnapshot returns the cached full pending mailbox
// response for an SO when it is available and valid.
func (a *ProviderAccount) getPendingMailboxResponseSnapshot(
	soID string,
) (*api.GetMailboxResponse, bool) {
	return a.getPendingMailboxCache().ResponseSnapshot(soID)
}

// getPendingMailboxResponseCached returns the cached full pending mailbox
// response, seeding it once via HTTP if needed.
func (a *ProviderAccount) getPendingMailboxResponseCached(
	ctx context.Context,
	soID string,
) (*api.GetMailboxResponse, error) {
	if !a.canAccessOwnerMailbox() {
		a.setPendingMailboxResponse(soID, &api.GetMailboxResponse{})
		return &api.GetMailboxResponse{}, nil
	}
	if resp, valid := a.getPendingMailboxResponseSnapshot(soID); valid {
		return resp, nil
	}

	var seed *seedflight.Seed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = a.getPendingMailboxSeedLocked(soID)
	})

	if err := seed.Run(ctx, &a.accountBcast, func(ctx context.Context) error {
		return a.syncPendingMailboxEntries(ctx, soID)
	}); err != nil {
		return nil, err
	}

	if resp, valid := a.getPendingMailboxResponseSnapshot(soID); valid {
		return resp, nil
	}
	return &api.GetMailboxResponse{}, nil
}

// canAccessOwnerMailbox returns true when cached account state permits owner
// mailbox reads and processing.
func (a *ProviderAccount) canAccessOwnerMailbox() bool {
	return a.canMutateCloudObjects()
}

// ApplyMailboxEntryEvent merges a mailbox entry event into the pending cache.
// Entries with terminal status (not "pending") are removed from the pending
// list and their terminal status is mirrored into the mailbox request tracker.
func (a *ProviderAccount) ApplyMailboxEntryEvent(
	soID string,
	entry *api.MailboxEntry,
	updatedAt int64,
) {
	track, changed := a.getPendingMailboxCache().ApplyEvent(soID, entry, updatedAt)
	if changed {
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	if track.Status != "" {
		a.TrackMailboxRequest(soID, track.InviteID, track.PeerID, track.Status)
	}
}

// InvalidatePendingMailboxEntries marks every cached pending mailbox entry as
// stale without dropping the current snapshot. The next
// GetPendingMailboxEntriesCached call re-seeds via singleflight, while
// existing snapshot readers keep their view until the fresh seed lands.
// Used on session WS reconnect to cover any events missed during the gap.
func (a *ProviderAccount) InvalidatePendingMailboxEntries() {
	if a.getPendingMailboxCache().InvalidateAll() {
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
}

// RemovePendingMailboxEntry removes a processed mailbox entry from the cached pending set.
func (a *ProviderAccount) RemovePendingMailboxEntry(soID string, entryID int64) {
	if a.getPendingMailboxCache().RemoveEntry(soID, entryID) {
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
}

// syncPendingMailboxEntries fetches and stores pending mailbox metadata for an SO.
func (a *ProviderAccount) syncPendingMailboxEntries(
	ctx context.Context,
	soID string,
) error {
	store := a.getPendingMailboxCache()
	changed, err := store.Sync(
		ctx,
		soID,
		a.canAccessOwnerMailbox(),
		a.GetSessionClient(),
		isMailboxAccessDeniedCloudError,
	)
	if changed {
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	return err
}

// setPendingMailboxResponse stores the full pending mailbox cache for an SO.
func (a *ProviderAccount) setPendingMailboxResponse(
	soID string,
	resp *api.GetMailboxResponse,
) {
	if a.getPendingMailboxCache().SetResponse(soID, resp) {
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
}

// isMailboxAccessDeniedCloudError checks if a cloud error means the caller
// cannot perform owner-side mailbox access for the current lifecycle or role.
func isMailboxAccessDeniedCloudError(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) {
		if ce.StatusCode != 403 {
			return false
		}
		switch ce.Code {
		case "account_read_only", "insufficient_role", "rbac_denied", "subscription_required":
			return true
		}
	}
	return false
}
