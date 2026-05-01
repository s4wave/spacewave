package provider_spacewave

import (
	"time"

	"github.com/aperturerobotics/util/broadcast"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
)

// SyncTelemetryUploadPhase describes upload-side Spacewave sync activity.
type SyncTelemetryUploadPhase int

const (
	// SyncTelemetryUploadPhaseIdle means no upload work is known.
	SyncTelemetryUploadPhaseIdle SyncTelemetryUploadPhase = iota
	// SyncTelemetryUploadPhaseDirtyPending means dirty blocks are waiting to upload.
	SyncTelemetryUploadPhaseDirtyPending
	// SyncTelemetryUploadPhasePushing means a packfile push is in flight.
	SyncTelemetryUploadPhasePushing
	// SyncTelemetryUploadPhaseError means upload sync has a recorded error.
	SyncTelemetryUploadPhaseError
)

// SyncTelemetrySnapshot describes Spacewave cloud sync activity.
type SyncTelemetrySnapshot struct {
	// UploadPhase is the current aggregate upload phase.
	UploadPhase SyncTelemetryUploadPhase
	// PendingUploadBytes is the approximate dirty upload backlog in bytes.
	PendingUploadBytes int64
	// PendingUploadCount is the approximate count of dirty upload items.
	PendingUploadCount int
	// ActiveUploadBytes is the approximate number of bytes currently being pushed.
	ActiveUploadBytes int64
	// ActiveUploadTransferredBytes is the approximate in-flight upload bytes sent.
	ActiveUploadTransferredBytes int64
	// InFlightPushes is the number of active packfile push requests.
	InFlightPushes int
	// PushCount is the number of completed packfile push requests.
	PushCount uint64
	// PushedBytes is the number of completed pushed packfile bytes.
	PushedBytes int64
	// DedupedUploadCount is the number of dirty blocks skipped because they already exist remotely.
	DedupedUploadCount uint64
	// DedupedUploadBytes is the number of dirty block bytes skipped because they already exist remotely.
	DedupedUploadBytes int64
	// PullActiveCount is the number of active sync-pull requests.
	PullActiveCount int
	// InFlightFetches is the number of active packfile range fetches.
	InFlightFetches int
	// FetchCount is the number of completed packfile range fetches.
	FetchCount uint64
	// FetchedBytes is the number of fetched packfile range bytes.
	FetchedBytes int64
	// RangeRequestCount is the number of completed packfile range requests.
	RangeRequestCount uint64
	// RangeResponseBytes is the number of bytes returned by range responses.
	RangeResponseBytes int64
	// IndexTailFetchCount is the number of completed index-tail range fetches.
	IndexTailFetchCount uint64
	// IndexTailFetchBytes is the number of requested index-tail range bytes.
	IndexTailFetchBytes int64
	// IndexTailResponseBytes is the number of bytes returned by index-tail range responses.
	IndexTailResponseBytes int64
	// FullResponseFallbackCount is the number of range requests served by a full response.
	FullResponseFallbackCount uint64
	// FullResponseFallbackBytes is the number of discarded prefix bytes from full responses.
	FullResponseFallbackBytes int64
	// LastFullResponseFallback is the largest recent full-response prefix discard.
	LastFullResponseFallback int64
	// LastFetchAt is the latest completed packfile range fetch time.
	LastFetchAt time.Time
	// ManifestEntries is the number of manifest pack entries.
	ManifestEntries int
	// PackBlockCountTotal is the sum of manifest entry block counts.
	PackBlockCountTotal uint64
	// PackBlockCountMin is the smallest manifest entry block count.
	PackBlockCountMin uint64
	// PackBlockCountMax is the largest manifest entry block count.
	PackBlockCountMax uint64
	// PackSizeBytesTotal is the sum of manifest entry pack sizes.
	PackSizeBytesTotal uint64
	// PackSizeBytesMin is the smallest manifest entry pack size.
	PackSizeBytesMin uint64
	// PackSizeBytesMax is the largest manifest entry pack size.
	PackSizeBytesMax uint64
	// BloomFilterCount is the number of entries with valid bloom metadata.
	BloomFilterCount int
	// BloomMissingCount is the number of entries with missing bloom metadata.
	BloomMissingCount int
	// BloomInvalidCount is the number of entries with malformed bloom metadata.
	BloomInvalidCount int
	// BloomParameterShapeCount is the summed per-store count of bloom parameter shapes.
	BloomParameterShapeCount int
	// BloomMaxFalsePositiveRate is the highest estimated bloom false-positive rate.
	BloomMaxFalsePositiveRate float64
	// BloomRiskPackCount is the number of packs above the bloom false-positive target.
	BloomRiskPackCount int
	// LookupCount is the number of pack lookups.
	LookupCount uint64
	// CandidatePacks is the total number of manifest candidates selected by lookups.
	CandidatePacks uint64
	// OpenedPacks is the total number of candidate packs opened by lookups.
	OpenedPacks uint64
	// NegativePacks is the total number of opened candidates that missed.
	NegativePacks uint64
	// TargetHits is the total number of lookups that found the target block.
	TargetHits uint64
	// LastCandidatePacks is the candidate count from the latest lookup.
	LastCandidatePacks int
	// LastOpenedPacks is the opened pack count from the latest lookup.
	LastOpenedPacks int
	// LastNegativePacks is the negative pack count from the latest lookup.
	LastNegativePacks int
	// LastTargetHit is true when the latest lookup found its target.
	LastTargetHit bool
	// IndexCacheHits is the number of pack index-tail cache hits.
	IndexCacheHits uint64
	// IndexCacheMisses is the number of pack index-tail cache misses.
	IndexCacheMisses uint64
	// IndexCacheReadErrors is the number of pack index-tail cache read errors.
	IndexCacheReadErrors uint64
	// IndexCacheWriteErrors is the number of pack index-tail cache write errors.
	IndexCacheWriteErrors uint64
	// RemoteIndexLoads is the number of remote pack index-tail loads.
	RemoteIndexLoads uint64
	// RemoteIndexBytes is the number of remote pack index-tail bytes fetched.
	RemoteIndexBytes int64
	// LastRemoteIndexBytes is the latest remote pack index-tail load byte count.
	LastRemoteIndexBytes int64
	// LastPushAt is the latest completed packfile push time.
	LastPushAt time.Time
	// LastPullAt is the latest completed sync-pull time.
	LastPullAt time.Time
	// LastActivityAt is the latest push, pull, or fetch activity time.
	LastActivityAt time.Time
	// LastPushError is the latest packfile push error.
	LastPushError string
	// LastPushErrorAt is the latest packfile push error time.
	LastPushErrorAt time.Time
	// LastPullError is the latest sync-pull error.
	LastPullError string
	// LastPullErrorAt is the latest sync-pull error time.
	LastPullErrorAt time.Time
	// LastError is the latest push or pull error.
	LastError string
	// LastErrorAt is the latest push or pull error time.
	LastErrorAt time.Time
	// StoreCount is the number of registered block stores.
	StoreCount int
}

type syncTelemetryFetchStatsProvider interface {
	SnapshotStats() packfile_store.PackfileStoreStats
}

type syncTelemetryStatsChangedProvider interface {
	SetStatsChangedCallback(func())
}

type syncTelemetryState struct {
	fetchStats syncTelemetryFetchStatsProvider

	pendingUploadBytes int64
	pendingUploadCount int
	activeUploadBytes  int64
	activeUploadSent   int64
	inFlightPushes     int
	pushCount          uint64
	pushedBytes        int64
	dedupedUploadCount uint64
	dedupedUploadBytes int64
	pullActiveCount    int
	lastPushAt         time.Time
	lastPullAt         time.Time
	lastActivityAt     time.Time
	lastPushError      string
	lastPushErrorAt    time.Time
	lastPullError      string
	lastPullErrorAt    time.Time
}

// GetSyncTelemetryBroadcast returns the broadcast guarding Spacewave sync telemetry.
func (a *ProviderAccount) GetSyncTelemetryBroadcast() *broadcast.Broadcast {
	return &a.syncTelemetryBcast
}

// GetSyncTelemetrySnapshot returns aggregate Spacewave sync telemetry.
func (a *ProviderAccount) GetSyncTelemetrySnapshot() SyncTelemetrySnapshot {
	states := a.cloneSyncTelemetryStates()
	return buildSyncTelemetrySnapshot(states)
}

// GetSyncTelemetrySnapshotWithWait returns aggregate sync telemetry and its wait channel.
func (a *ProviderAccount) GetSyncTelemetrySnapshotWithWait() (SyncTelemetrySnapshot, <-chan struct{}) {
	var ch <-chan struct{}
	var states []syncTelemetryState
	a.syncTelemetryBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		states = a.cloneSyncTelemetryStatesLocked()
	})
	return buildSyncTelemetrySnapshot(states), ch
}

func buildSyncTelemetrySnapshot(states []syncTelemetryState) SyncTelemetrySnapshot {
	snap := SyncTelemetrySnapshot{
		StoreCount: len(states),
	}
	for _, state := range states {
		snap.PendingUploadBytes += state.pendingUploadBytes
		snap.PendingUploadCount += state.pendingUploadCount
		snap.ActiveUploadBytes += state.activeUploadBytes
		snap.ActiveUploadTransferredBytes += state.activeUploadSent
		snap.InFlightPushes += state.inFlightPushes
		snap.PushCount += state.pushCount
		snap.PushedBytes += state.pushedBytes
		snap.DedupedUploadCount += state.dedupedUploadCount
		snap.DedupedUploadBytes += state.dedupedUploadBytes
		snap.PullActiveCount += state.pullActiveCount
		snap.LastPushAt = maxTime(snap.LastPushAt, state.lastPushAt)
		snap.LastPullAt = maxTime(snap.LastPullAt, state.lastPullAt)
		snap.LastActivityAt = maxTime(snap.LastActivityAt, state.lastActivityAt)
		if state.lastPushError != "" {
			snap.LastPushError = state.lastPushError
			snap.LastPushErrorAt = state.lastPushErrorAt
			if snap.LastErrorAt.Before(state.lastPushErrorAt) {
				snap.LastError = state.lastPushError
				snap.LastErrorAt = state.lastPushErrorAt
			}
		}
		if state.lastPullError != "" {
			snap.LastPullError = state.lastPullError
			snap.LastPullErrorAt = state.lastPullErrorAt
			if snap.LastErrorAt.Before(state.lastPullErrorAt) {
				snap.LastError = state.lastPullError
				snap.LastErrorAt = state.lastPullErrorAt
			}
		}
		if state.fetchStats == nil {
			continue
		}
		stats := state.fetchStats.SnapshotStats()
		snap.InFlightFetches += stats.InFlightFetches
		snap.FetchCount += stats.FetchCount
		snap.FetchedBytes += stats.FetchedBytes
		snap.RangeRequestCount += stats.RangeRequestCount
		snap.RangeResponseBytes += stats.RangeResponseBytes
		snap.IndexTailFetchCount += stats.IndexTailFetchCount
		snap.IndexTailFetchBytes += stats.IndexTailFetchBytes
		snap.IndexTailResponseBytes += stats.IndexTailResponseBytes
		snap.FullResponseFallbackCount += stats.FullResponseFallbackCount
		snap.FullResponseFallbackBytes += stats.FullResponseFallbackBytes
		if snap.LastFullResponseFallback < stats.LastFullResponseFallback {
			snap.LastFullResponseFallback = stats.LastFullResponseFallback
		}
		hadManifestEntries := snap.ManifestEntries != 0
		snap.ManifestEntries += stats.ManifestEntries
		snap.PackBlockCountTotal += stats.PackBlockCountTotal
		snap.PackSizeBytesTotal += stats.PackSizeBytesTotal
		if stats.ManifestEntries != 0 {
			if !hadManifestEntries || stats.PackBlockCountMin < snap.PackBlockCountMin {
				snap.PackBlockCountMin = stats.PackBlockCountMin
			}
			if snap.PackBlockCountMax < stats.PackBlockCountMax {
				snap.PackBlockCountMax = stats.PackBlockCountMax
			}
			if !hadManifestEntries || stats.PackSizeBytesMin < snap.PackSizeBytesMin {
				snap.PackSizeBytesMin = stats.PackSizeBytesMin
			}
			if snap.PackSizeBytesMax < stats.PackSizeBytesMax {
				snap.PackSizeBytesMax = stats.PackSizeBytesMax
			}
		}
		snap.BloomFilterCount += stats.BloomFilterCount
		snap.BloomMissingCount += stats.BloomMissingCount
		snap.BloomInvalidCount += stats.BloomInvalidCount
		snap.BloomParameterShapeCount += stats.BloomParameterShapeCount
		if snap.BloomMaxFalsePositiveRate < stats.BloomMaxFalsePositiveRate {
			snap.BloomMaxFalsePositiveRate = stats.BloomMaxFalsePositiveRate
		}
		snap.BloomRiskPackCount += stats.BloomRiskPackCount
		snap.LookupCount += stats.LookupCount
		snap.CandidatePacks += stats.CandidatePacks
		snap.OpenedPacks += stats.OpenedPacks
		snap.NegativePacks += stats.NegativePacks
		snap.TargetHits += stats.TargetHits
		snap.LastCandidatePacks += stats.LastCandidatePacks
		snap.LastOpenedPacks += stats.LastOpenedPacks
		snap.LastNegativePacks += stats.LastNegativePacks
		snap.LastTargetHit = snap.LastTargetHit || stats.LastTargetHit
		snap.IndexCacheHits += stats.IndexCacheHits
		snap.IndexCacheMisses += stats.IndexCacheMisses
		snap.IndexCacheReadErrors += stats.IndexCacheReadErrors
		snap.IndexCacheWriteErrors += stats.IndexCacheWriteErrors
		snap.RemoteIndexLoads += stats.RemoteIndexLoads
		snap.RemoteIndexBytes += stats.RemoteIndexBytes
		if snap.LastRemoteIndexBytes < stats.LastRemoteIndexBytes {
			snap.LastRemoteIndexBytes = stats.LastRemoteIndexBytes
		}
		snap.LastFetchAt = maxTime(snap.LastFetchAt, stats.LastFetchAt)
		snap.LastActivityAt = maxTime(snap.LastActivityAt, stats.LastFetchAt)
	}
	snap.UploadPhase = syncTelemetryUploadPhase(snap)
	return snap
}

func (a *ProviderAccount) cloneSyncTelemetryStates() []syncTelemetryState {
	var states []syncTelemetryState
	a.syncTelemetryBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		states = a.cloneSyncTelemetryStatesLocked()
	})
	return states
}

func (a *ProviderAccount) cloneSyncTelemetryStatesLocked() []syncTelemetryState {
	if len(a.syncTelemetry) == 0 {
		return nil
	}
	states := make([]syncTelemetryState, 0, len(a.syncTelemetry))
	for _, state := range a.syncTelemetry {
		if state == nil {
			continue
		}
		states = append(states, *state)
	}
	return states
}

func (a *ProviderAccount) registerSyncTelemetryStore(
	bstoreID string,
	fetchStats syncTelemetryFetchStatsProvider,
) func() {
	if bstoreID == "" {
		return func() {}
	}
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.syncTelemetry == nil {
			a.syncTelemetry = make(map[string]*syncTelemetryState)
		}
		state := a.syncTelemetry[bstoreID]
		if state == nil {
			state = &syncTelemetryState{}
			a.syncTelemetry[bstoreID] = state
		}
		state.fetchStats = fetchStats
		broadcast()
	})
	if notifier, ok := fetchStats.(syncTelemetryStatsChangedProvider); ok {
		notifier.SetStatsChangedCallback(a.broadcastSyncTelemetry)
	}
	return func() {
		if notifier, ok := fetchStats.(syncTelemetryStatsChangedProvider); ok {
			notifier.SetStatsChangedCallback(nil)
		}
		a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if a.syncTelemetry == nil {
				return
			}
			delete(a.syncTelemetry, bstoreID)
			broadcast()
		})
	}
}

func (a *ProviderAccount) broadcastSyncTelemetry() {
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

func (a *ProviderAccount) setSyncTelemetryPending(bstoreID string, bytes int64, count int) {
	if bytes < 0 {
		bytes = 0
	}
	if count < 0 {
		count = 0
	}
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		state.pendingUploadBytes = bytes
		state.pendingUploadCount = count
		broadcast()
	})
}

func (a *ProviderAccount) addSyncTelemetryDirty(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	now := time.Now()
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		state.pendingUploadBytes += bytes
		state.pendingUploadCount++
		state.lastActivityAt = now
		broadcast()
	})
}

func (a *ProviderAccount) startSyncTelemetryPush(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		state.inFlightPushes++
		state.activeUploadBytes += bytes
		broadcast()
	})
}

func (a *ProviderAccount) setSyncTelemetryPushProgress(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		if bytes > state.activeUploadBytes {
			bytes = state.activeUploadBytes
		}
		if state.activeUploadSent == bytes {
			return
		}
		state.activeUploadSent = bytes
		broadcast()
	})
}

func (a *ProviderAccount) finishSyncTelemetryPush(bstoreID string, bytes int64, err error) {
	if bytes < 0 {
		bytes = 0
	}
	now := time.Now()
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		if state.inFlightPushes > 0 {
			state.inFlightPushes--
		}
		state.activeUploadBytes -= bytes
		if state.activeUploadBytes < 0 {
			state.activeUploadBytes = 0
		}
		if state.activeUploadBytes == 0 {
			state.activeUploadSent = 0
		}
		if err != nil {
			state.lastPushError = err.Error()
			state.lastPushErrorAt = now
		}
		if err == nil {
			state.pushCount++
			state.pushedBytes += bytes
			state.lastPushAt = now
			state.lastPushError = ""
			state.lastPushErrorAt = time.Time{}
		}
		state.lastActivityAt = now
		broadcast()
	})
}

func (a *ProviderAccount) addSyncTelemetryDeduped(bstoreID string, bytes int64, count int) {
	if bytes < 0 {
		bytes = 0
	}
	if count < 0 {
		count = 0
	}
	now := time.Now()
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		state.dedupedUploadBytes += bytes
		state.dedupedUploadCount += uint64(count)
		state.lastActivityAt = now
		broadcast()
	})
}

func (a *ProviderAccount) startSyncTelemetryPull(bstoreID string) {
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		state.pullActiveCount++
		broadcast()
	})
}

func (a *ProviderAccount) finishSyncTelemetryPull(bstoreID string, err error) {
	now := time.Now()
	a.syncTelemetryBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := a.getOrCreateSyncTelemetryStateLocked(bstoreID)
		if state.pullActiveCount > 0 {
			state.pullActiveCount--
		}
		if err != nil {
			state.lastPullError = err.Error()
			state.lastPullErrorAt = now
		}
		if err == nil {
			state.lastPullAt = now
			state.lastPullError = ""
			state.lastPullErrorAt = time.Time{}
		}
		state.lastActivityAt = now
		broadcast()
	})
}

func (a *ProviderAccount) getOrCreateSyncTelemetryStateLocked(bstoreID string) *syncTelemetryState {
	if a.syncTelemetry == nil {
		a.syncTelemetry = make(map[string]*syncTelemetryState)
	}
	state := a.syncTelemetry[bstoreID]
	if state == nil {
		state = &syncTelemetryState{}
		a.syncTelemetry[bstoreID] = state
	}
	return state
}

func syncTelemetryUploadPhase(snap SyncTelemetrySnapshot) SyncTelemetryUploadPhase {
	if snap.LastPushError != "" {
		return SyncTelemetryUploadPhaseError
	}
	if snap.InFlightPushes > 0 {
		return SyncTelemetryUploadPhasePushing
	}
	if snap.PendingUploadBytes > 0 || snap.PendingUploadCount > 0 {
		return SyncTelemetryUploadPhaseDirtyPending
	}
	return SyncTelemetryUploadPhaseIdle
}

func maxTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

// _ is a type assertion
var _ syncTelemetryFetchStatsProvider = ((*packfile_store.PackfileStore)(nil))
