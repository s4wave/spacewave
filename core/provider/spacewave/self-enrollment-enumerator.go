package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"

	"github.com/s4wave/spacewave/core/cdn"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// SelfEnrollmentSummary is the cached post-login self-enrollment predicate.
type SelfEnrollmentSummary struct {
	ids           []string
	generationKey string
	count         uint32
	loaded        bool
}

// selfEnrollmentEntry is the memoized verified state of one SharedObject. A
// nil config records that no verified config is cached.
type selfEnrollmentEntry struct {
	config    *sobject.SharedObjectConfig
	keyEpochs []*sobject.SOKeyEpoch
}

// RefreshSelfEnrollmentSummary recomputes the self-enrollment summary from
// the SharedObject list and the verified caches of its entries.
func (a *ProviderAccount) RefreshSelfEnrollmentSummary(ctx context.Context) (rerr error) {
	// Clear the summary when the cloud reports an unauthenticated error.
	defer func() {
		if clouderror.IsUnauth(rerr) {
			_ = a.setSelfEnrollmentSummary(nil)
		}
	}()

	// Read the cached shared object list when the controller is available.
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.soListCtr == nil {
		return a.setSelfEnrollmentSummary(nil)
	}
	list := a.soListCtr.GetValue()

	// Load the shared object list when it is not yet available.
	if list == nil {
		if !a.hasSharedObjectListAccess() {
			return a.setSelfEnrollmentSummary(&SelfEnrollmentSummary{loaded: true})
		}
		if err := a.EnsureSharedObjectListLoaded(ctx); err != nil {
			return err
		}
		list = a.soListCtr.GetValue()
		if list == nil {
			return a.setSelfEnrollmentSummary(nil)
		}
	}

	// Enumerate enrollment candidates for the current session and entity.
	sessionPeerID := a.GetCurrentSessionPeerID()
	if sessionPeerID == "" {
		return a.setSelfEnrollmentSummary(nil)
	}
	entityID, err := a.GetSelfEntityID(ctx)
	if err != nil {
		return err
	}
	summary, err := a.enumerateSelfEnrollmentCandidates(
		ctx,
		list,
		sessionPeerID,
		entityID,
	)
	if err != nil {
		return err
	}
	return a.setSelfEnrollmentSummary(summary)
}

// GetSelfEnrollmentSummary returns a copy of the current summary, or nil when
// none is computed. Call it inside ReadAccountState.
func (a *ProviderAccount) GetSelfEnrollmentSummary() *SelfEnrollmentSummary {
	return a.state.selfEnrollmentSummary.clone()
}

// GetSelfEnrollmentSkippedGenerationKey returns the generation key the user
// skipped. The caller holds the account broadcast lock.
func (a *ProviderAccount) GetSelfEnrollmentSkippedGenerationKey() string {
	return a.state.selfEnrollmentSkippedGenerationKey
}

// SetSelfEnrollmentSkippedGenerationKey records the generation key the user
// skipped and broadcasts when it changed.
func (a *ProviderAccount) SetSelfEnrollmentSkippedGenerationKey(key string) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.selfEnrollmentSkippedGenerationKey == key {
			return
		}
		a.state.selfEnrollmentSkippedGenerationKey = key
		broadcast()
	})
}

// GetIDs returns the shared object IDs needing self-enrollment. A nil summary
// has none. Its getters answer the zero value, as protobuf getters do.
func (s *SelfEnrollmentSummary) GetIDs() []string {
	if s == nil {
		return nil
	}
	return slices.Clone(s.ids)
}

// GetGenerationKey returns the generation key for the current pending set.
func (s *SelfEnrollmentSummary) GetGenerationKey() string {
	if s == nil {
		return ""
	}
	return s.generationKey
}

// GetCount returns the number of shared objects needing self-enrollment.
func (s *SelfEnrollmentSummary) GetCount() uint32 {
	if s == nil {
		return 0
	}
	return s.count
}

// GetLoaded returns whether every shared-object list entry was evaluated.
func (s *SelfEnrollmentSummary) GetLoaded() bool {
	return s != nil && s.loaded
}

// enumerateSelfEnrollmentCandidates returns the summary of the listed
// SharedObjects the entity can read whose current key epoch lacks the session
// peer. A SharedObject without a verified config is a candidate.
func (a *ProviderAccount) enumerateSelfEnrollmentCandidates(
	ctx context.Context,
	list *sobject.SharedObjectList,
	sessionPeerID peer.ID,
	entityID string,
) (*SelfEnrollmentSummary, error) {
	// Check the context and collect the eligible SharedObject IDs.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var soIDs []string
	for _, entry := range list.GetSharedObjects() {
		if !a.isSelfEnrollmentExcludedSharedObject(entry) {
			soIDs = append(soIDs, entry.GetRef().GetProviderResourceRef().GetId())
		}
	}

	// Load the verified state of each SharedObject, reading the store only
	// for those not memoized.
	entries, err := a.loadSelfEnrollmentEntries(ctx, soIDs)
	if err != nil {
		return nil, err
	}

	// Collect the candidates the entity can read but the session cannot.
	var ids []string
	for _, soID := range soIDs {
		entry := entries[soID]
		if entry.config == nil {
			ids = append(ids, soID)
			continue
		}
		role := readableParticipantRoleForEntity(entry.config, entityID)
		if !sobject.CanReadState(role) {
			continue
		}
		if peerEnrolledInCurrentEpoch(entry.keyEpochs, sessionPeerID.String()) {
			continue
		}
		ids = append(ids, soID)
	}

	// Sort the IDs and build the generation key.
	slices.Sort(ids)
	return &SelfEnrollmentSummary{
		ids:           ids,
		generationKey: buildSelfEnrollmentGenerationKey(ids, sessionPeerID),
		count:         uint32(len(ids)), //nolint:gosec // ids is the bounded in-memory enumeration result.
		loaded:        true,
	}, nil
}

// loadSelfEnrollmentEntries returns the verified state of each of soIDs. It
// reads the store only for entries absent from the memo, and memoizes the
// result unless a verified cache changed during the load.
func (a *ProviderAccount) loadSelfEnrollmentEntries(
	ctx context.Context,
	soIDs []string,
) (map[string]*selfEnrollmentEntry, error) {
	// Take the memoized entries and the revision they belong to.
	entries := make(map[string]*selfEnrollmentEntry, len(soIDs))
	var rev uint64
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rev = a.state.selfEnrollmentEntriesRev
		for _, soID := range soIDs {
			if entry, ok := a.state.selfEnrollmentEntries[soID]; ok {
				entries[soID] = entry
			}
		}
	})

	// Load the absent entries from the store.
	for _, soID := range soIDs {
		if _, ok := entries[soID]; ok {
			continue
		}
		cache, err := a.loadVerifiedSOStateCache(ctx, soID)
		if err != nil {
			return nil, err
		}
		entry := &selfEnrollmentEntry{}
		if config := cache.GetCurrentConfig(); config != nil {
			entry.config = config
			entry.keyEpochs = cache.GetKeyEpochs()
		}
		entries[soID] = entry
	}

	// Memoize a copy of the listed entries, since the caller reads the
	// returned map unlocked, unless an invalidation raced the load.
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.selfEnrollmentEntriesRev == rev {
			a.state.selfEnrollmentEntries = maps.Clone(entries)
		}
	})
	return entries, nil
}

// invalidateSelfEnrollmentEntry drops the memoized verified state of soID
// after its verified cache changed, and schedules a summary refresh.
func (a *ProviderAccount) invalidateSelfEnrollmentEntry(soID string) {
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(a.state.selfEnrollmentEntries, soID)
		a.state.selfEnrollmentEntriesRev++
	})
	a.selfEnrollmentRefresh.Trigger()
}

// refreshSelfEnrollmentSummary recomputes the summary, logging a failure.
func (a *ProviderAccount) refreshSelfEnrollmentSummary(ctx context.Context) {
	if err := a.RefreshSelfEnrollmentSummary(ctx); err != nil {
		// note: this fork of logrus dereferences the entry, so le must be set.
		if a.le != nil {
			a.le.WithError(err).Warn("failed to refresh self-enrollment summary")
		}
	}
}

// setSelfEnrollmentSummary stores summary and auto-starts a run when the
// pending set changed.
func (a *ProviderAccount) setSelfEnrollmentSummary(summary *SelfEnrollmentSummary) error {
	// Store the summary when it changed.
	next := summary.clone()
	var shouldRun bool
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if equalSelfEnrollmentSummary(a.state.selfEnrollmentSummary, next) {
			return
		}
		a.state.selfEnrollmentSummary = next
		shouldRun = a.shouldAutoStartSelfEnrollmentRunLocked(next)
		broadcast()
	})

	// Start the run outside the lock.
	if shouldRun {
		a.startSelfEnrollmentRunFromSummary(next)
	}
	return nil
}

// clone returns a copy of s.
func (s *SelfEnrollmentSummary) clone() *SelfEnrollmentSummary {
	if s == nil {
		return nil
	}
	return &SelfEnrollmentSummary{
		ids:           slices.Clone(s.ids),
		generationKey: s.generationKey,
		count:         s.count,
		loaded:        s.loaded,
	}
}

// equalSelfEnrollmentSummary reports whether a and b hold the same summary.
func equalSelfEnrollmentSummary(a, b *SelfEnrollmentSummary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.generationKey == b.generationKey &&
		a.count == b.count &&
		a.loaded == b.loaded &&
		slices.Equal(a.ids, b.ids)
}

// isSelfEnrollmentExcludedSharedObject reports whether entry is outside
// self-enrollment: unaddressable, CDN, deleted or publicly readable.
func (a *ProviderAccount) isSelfEnrollmentExcludedSharedObject(
	entry *sobject.SharedObjectListEntry,
) bool {
	// Exclude entries without a usable provider resource ref.
	ref := entry.GetRef()
	if ref == nil || ref.GetProviderResourceRef() == nil {
		return true
	}

	// Exclude the CDN space and CDN-typed bodies.
	soID := ref.GetProviderResourceRef().GetId()
	if soID == "" || soID == cdn.SpaceID() {
		return true
	}
	bodyType := entry.GetMeta().GetBodyType()
	if bodyType == cdn_sharedobject.CdnBodyType || bodyType == "cdn" {
		return true
	}
	metadata, status := a.getSharedObjectMetadataSnapshot(soID)
	if status == sharedObjectMetadataDeleted {
		return true
	}
	return status == sharedObjectMetadataValid && metadata.GetPublicRead()
}

// buildSelfEnrollmentGenerationKey hashes ids and sessionPeerID into the key
// identifying one pending set, or returns an empty string when ids is empty.
func buildSelfEnrollmentGenerationKey(ids []string, sessionPeerID peer.ID) string {
	// Hash each ID and the session peer into a generation key.
	if len(ids) == 0 {
		return ""
	}
	h := sha256.New()
	for _, id := range ids {
		_, _ = h.Write([]byte(id))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte(sessionPeerID))
	return hex.EncodeToString(h.Sum(nil))
}
