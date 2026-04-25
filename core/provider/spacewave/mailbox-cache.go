package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// pendingMailboxState stores the cached pending mailbox metadata for one SO.
type pendingMailboxState struct {
	// response is the cached pending mailbox response for one SO. Nil means the
	// cache currently only has the reduced UI metadata view, so full-entry
	// callers like owner-side auto-processing must still seed once.
	response *api.GetMailboxResponse
	// entries is the reduced UI-facing mailbox metadata derived from response.
	entries []*s4wave_provider_spacewave.MailboxEntryInfo
	// valid indicates entries/response reflect the latest known pending mailbox
	// snapshot for this SO.
	valid bool
	// entryVersion tracks the last applied DO-side updatedAt per entry id.
	// Events with updatedAt less than or equal to the recorded version are
	// discarded as out-of-order replays.
	entryVersion map[int64]int64
	// seed coordinates concurrent callers around a single seed HTTP fetch.
	// Guarded by accountBcast like the rest of the state.
	seed providerSeed
}

// GetPendingMailboxEntriesSnapshot returns the cached pending mailbox entries for an SO.
func (a *ProviderAccount) GetPendingMailboxEntriesSnapshot(
	soID string,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, bool) {
	var entries []*s4wave_provider_spacewave.MailboxEntryInfo
	var valid bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.pendingMailboxEntries == nil {
			return
		}
		state := a.state.pendingMailboxEntries[soID]
		if state == nil {
			return
		}
		entries = clonePendingMailboxEntries(state.entries)
		valid = state.valid
	})
	return entries, valid
}

// GetPendingMailboxEntriesCached returns the cached pending mailbox entries,
// seeding once via HTTP if the cache is not yet valid. Concurrent callers
// share a single in-flight seed request via providerSeed; subsequent calls
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

	var seed *providerSeed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = &a.getOrCreatePendingMailboxStateLocked(soID).seed
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
	var resp *api.GetMailboxResponse
	var valid bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.pendingMailboxEntries == nil {
			return
		}
		state := a.state.pendingMailboxEntries[soID]
		if state == nil || !state.valid || state.response == nil {
			return
		}
		resp = state.response.CloneVT()
		valid = true
	})
	return resp, valid
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

	var seed *providerSeed
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seed = &a.getOrCreatePendingMailboxStateLocked(soID).seed
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
// The event carries the full entry plus a DO-side updatedAt; replays with
// updatedAt <= the last applied version for that entry id are discarded.
// Entries with terminal status (not "pending") are removed from the pending
// list and their terminal status is mirrored into the mailbox request tracker.
func (a *ProviderAccount) ApplyMailboxEntryEvent(
	soID string,
	entry *api.MailboxEntry,
	updatedAt int64,
) {
	if soID == "" || entry == nil {
		return
	}
	id := entry.GetId()
	if id == 0 {
		return
	}
	info := &s4wave_provider_spacewave.MailboxEntryInfo{
		Id:        id,
		InviteId:  entry.GetInviteId(),
		PeerId:    entry.GetPeerId(),
		Status:    entry.GetStatus(),
		CreatedAt: entry.GetCreatedAt(),
		AccountId: entry.GetAccountId(),
		EntityId:  entry.GetEntityId(),
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreatePendingMailboxStateLocked(soID)
		if state.entryVersion == nil {
			state.entryVersion = make(map[int64]int64)
		}
		if last, ok := state.entryVersion[id]; ok && updatedAt != 0 && updatedAt <= last {
			return
		}
		state.entryVersion[id] = updatedAt
		nextEntries, entriesChanged := upsertPendingMailboxEntryLocked(state.entries, info)
		nextResp, respChanged := upsertPendingMailboxResponseLocked(state.response, entry)
		if !entriesChanged && !respChanged {
			// Still mark the cache valid even if the set is unchanged, e.g.
			// when a terminal event arrives before the cache was seeded.
			if !state.valid {
				state.entries = nextEntries
				state.response = nextResp
				state.valid = true
				broadcast()
			}
			return
		}
		state.entries = nextEntries
		state.response = nextResp
		state.valid = true
		broadcast()
	})
	if status := entry.GetStatus(); status != "" && status != "pending" {
		a.TrackMailboxRequest(soID, entry.GetInviteId(), entry.GetPeerId(), status)
	}
}

// upsertPendingMailboxEntryLocked merges an entry into the pending list by id.
// Terminal entries (status != "pending") are removed. Returns the new slice
// and a changed flag indicating whether the set was modified.
func upsertPendingMailboxEntryLocked(
	current []*s4wave_provider_spacewave.MailboxEntryInfo,
	info *s4wave_provider_spacewave.MailboxEntryInfo,
) ([]*s4wave_provider_spacewave.MailboxEntryInfo, bool) {
	id := info.GetId()
	terminal := info.GetStatus() != "" && info.GetStatus() != "pending"
	found := -1
	for i, e := range current {
		if e.GetId() == id {
			found = i
			break
		}
	}
	if terminal {
		if found < 0 {
			return current, false
		}
		next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(current)-1)
		for i, e := range current {
			if i == found {
				continue
			}
			next = append(next, clonePendingMailboxEntry(e))
		}
		return next, true
	}
	if found >= 0 {
		existing := current[found]
		if existing.GetInviteId() == info.GetInviteId() &&
			existing.GetPeerId() == info.GetPeerId() &&
			existing.GetStatus() == info.GetStatus() &&
			existing.GetCreatedAt() == info.GetCreatedAt() {
			return current, false
		}
		next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, len(current))
		for i, e := range current {
			if i == found {
				next[i] = clonePendingMailboxEntry(info)
				continue
			}
			next[i] = clonePendingMailboxEntry(e)
		}
		return next, true
	}
	next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(current)+1)
	for _, e := range current {
		next = append(next, clonePendingMailboxEntry(e))
	}
	next = append(next, clonePendingMailboxEntry(info))
	return next, true
}

// upsertPendingMailboxResponseLocked merges an entry into the cached full
// pending mailbox response by id. Terminal entries (status != "pending") are
// removed. Returns the new response and a changed flag.
func upsertPendingMailboxResponseLocked(
	current *api.GetMailboxResponse,
	entry *api.MailboxEntry,
) (*api.GetMailboxResponse, bool) {
	if entry == nil {
		return current, false
	}
	next := &api.GetMailboxResponse{}
	if current != nil {
		next = current.CloneVT()
	}
	id := entry.GetId()
	terminal := entry.GetStatus() != "" && entry.GetStatus() != "pending"
	found := -1
	for i, candidate := range next.GetEntries() {
		if candidate.GetId() == id {
			found = i
			break
		}
	}
	if terminal {
		if found < 0 {
			return next, false
		}
		filtered := make([]*api.MailboxEntry, 0, len(next.GetEntries())-1)
		for i, candidate := range next.GetEntries() {
			if i == found {
				continue
			}
			filtered = append(filtered, candidate.CloneVT())
		}
		next.Entries = filtered
		return next, true
	}
	if found >= 0 {
		if next.GetEntries()[found].EqualVT(entry) {
			return next, false
		}
		next.Entries[found] = entry.CloneVT()
		return next, true
	}
	next.Entries = append(next.GetEntries(), entry.CloneVT())
	return next, true
}

// InvalidatePendingMailboxEntries marks every cached pending mailbox entry as
// stale without dropping the current snapshot. The next
// GetPendingMailboxEntriesCached call re-seeds via singleflight, while
// existing snapshot readers keep their view until the fresh seed lands.
// Used on session WS reconnect to cover any events missed during the gap.
func (a *ProviderAccount) InvalidatePendingMailboxEntries() {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if len(a.state.pendingMailboxEntries) == 0 {
			return
		}
		for _, state := range a.state.pendingMailboxEntries {
			if state == nil {
				continue
			}
			state.valid = false
			state.entryVersion = nil
		}
		broadcast()
	})
}

// RemovePendingMailboxEntry removes a processed mailbox entry from the cached pending set.
func (a *ProviderAccount) RemovePendingMailboxEntry(soID string, entryID int64) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreatePendingMailboxStateLocked(soID)
		if len(state.entries) == 0 && (state.response == nil || len(state.response.GetEntries()) == 0) {
			return
		}

		next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(state.entries))
		nextResp := &api.GetMailboxResponse{}
		if state.response != nil {
			nextResp = state.response.CloneVT()
		}
		changed := false
		for _, entry := range state.entries {
			if entry.GetId() == entryID {
				changed = true
				continue
			}
			next = append(next, clonePendingMailboxEntry(entry))
		}
		if state.response != nil {
			filtered := make([]*api.MailboxEntry, 0, len(state.response.GetEntries()))
			for _, entry := range state.response.GetEntries() {
				if entry.GetId() == entryID {
					changed = true
					continue
				}
				filtered = append(filtered, entry.CloneVT())
			}
			nextResp.Entries = filtered
		}
		if !changed {
			return
		}
		state.entries = next
		state.response = nextResp
		state.valid = true
		broadcast()
	})
}

// syncPendingMailboxEntries fetches and stores pending mailbox metadata for an SO.
func (a *ProviderAccount) syncPendingMailboxEntries(
	ctx context.Context,
	soID string,
) error {
	if !a.canAccessOwnerMailbox() {
		a.setPendingMailboxResponse(soID, &api.GetMailboxResponse{})
		return nil
	}

	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not ready")
	}

	resp, err := cli.GetMailboxEntries(ctx, soID)
	if err != nil {
		if isMailboxAccessDeniedCloudError(err) {
			a.setPendingMailboxResponse(soID, &api.GetMailboxResponse{})
			return nil
		}
		return err
	}
	a.setPendingMailboxResponse(soID, resp)
	return nil
}

// setPendingMailboxResponse stores the full pending mailbox cache for an SO.
func (a *ProviderAccount) setPendingMailboxResponse(
	soID string,
	resp *api.GetMailboxResponse,
) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreatePendingMailboxStateLocked(soID)
		nextResp := clonePendingMailboxResponse(resp)
		nextEntries := mailboxEntriesToProto(nextResp)
		if state.valid &&
			pendingMailboxEntriesEqual(state.entries, nextEntries) &&
			pendingMailboxResponseEqual(state.response, nextResp) {
			return
		}
		state.response = nextResp
		state.entries = nextEntries
		state.valid = true
		broadcast()
	})
}

// getOrCreatePendingMailboxStateLocked returns the mailbox cache state for an SO.
func (a *ProviderAccount) getOrCreatePendingMailboxStateLocked(
	soID string,
) *pendingMailboxState {
	if a.state.pendingMailboxEntries == nil {
		a.state.pendingMailboxEntries = make(map[string]*pendingMailboxState)
	}
	state := a.state.pendingMailboxEntries[soID]
	if state == nil {
		state = &pendingMailboxState{}
		a.state.pendingMailboxEntries[soID] = state
	}
	return state
}

// mailboxEntriesToProto converts the cloud mailbox response into cached metadata.
func mailboxEntriesToProto(
	resp *api.GetMailboxResponse,
) []*s4wave_provider_spacewave.MailboxEntryInfo {
	if resp == nil {
		return nil
	}
	entries := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(resp.GetEntries()))
	for _, entry := range resp.GetEntries() {
		if entry.GetStatus() != "pending" {
			continue
		}
		entries = append(entries, &s4wave_provider_spacewave.MailboxEntryInfo{
			Id:        entry.GetId(),
			InviteId:  entry.GetInviteId(),
			PeerId:    entry.GetPeerId(),
			Status:    entry.GetStatus(),
			CreatedAt: entry.GetCreatedAt(),
			AccountId: entry.GetAccountId(),
			EntityId:  entry.GetEntityId(),
		})
	}
	return entries
}

// clonePendingMailboxResponse clones the cached full pending mailbox response.
func clonePendingMailboxResponse(
	resp *api.GetMailboxResponse,
) *api.GetMailboxResponse {
	if resp == nil {
		return nil
	}
	return resp.CloneVT()
}

// clonePendingMailboxEntries clones the cached mailbox metadata slice.
func clonePendingMailboxEntries(
	entries []*s4wave_provider_spacewave.MailboxEntryInfo,
) []*s4wave_provider_spacewave.MailboxEntryInfo {
	if len(entries) == 0 {
		return nil
	}
	next := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(entries))
	for _, entry := range entries {
		next = append(next, clonePendingMailboxEntry(entry))
	}
	return next
}

// clonePendingMailboxEntry clones one cached mailbox metadata entry.
func clonePendingMailboxEntry(
	entry *s4wave_provider_spacewave.MailboxEntryInfo,
) *s4wave_provider_spacewave.MailboxEntryInfo {
	if entry == nil {
		return nil
	}
	return &s4wave_provider_spacewave.MailboxEntryInfo{
		Id:        entry.GetId(),
		InviteId:  entry.GetInviteId(),
		PeerId:    entry.GetPeerId(),
		Status:    entry.GetStatus(),
		CreatedAt: entry.GetCreatedAt(),
		AccountId: entry.GetAccountId(),
		EntityId:  entry.GetEntityId(),
	}
}

// pendingMailboxEntriesEqual compares cached mailbox metadata slices.
func pendingMailboxEntriesEqual(
	aEntries []*s4wave_provider_spacewave.MailboxEntryInfo,
	bEntries []*s4wave_provider_spacewave.MailboxEntryInfo,
) bool {
	if len(aEntries) != len(bEntries) {
		return false
	}
	for i := range aEntries {
		aEntry := aEntries[i]
		bEntry := bEntries[i]
		if aEntry == nil || bEntry == nil {
			if aEntry != bEntry {
				return false
			}
			continue
		}
		if aEntry.GetId() != bEntry.GetId() ||
			aEntry.GetInviteId() != bEntry.GetInviteId() ||
			aEntry.GetPeerId() != bEntry.GetPeerId() ||
			aEntry.GetStatus() != bEntry.GetStatus() ||
			aEntry.GetCreatedAt() != bEntry.GetCreatedAt() ||
			aEntry.GetAccountId() != bEntry.GetAccountId() ||
			aEntry.GetEntityId() != bEntry.GetEntityId() {
			return false
		}
	}
	return true
}

// pendingMailboxResponseEqual compares cached full mailbox responses.
func pendingMailboxResponseEqual(
	aResp *api.GetMailboxResponse,
	bResp *api.GetMailboxResponse,
) bool {
	if aResp == nil || bResp == nil {
		return aResp == bResp
	}
	return aResp.EqualVT(bResp)
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
