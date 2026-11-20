package synctelemetry

import (
	"slices"
	"time"
)

// UploadPhase describes upload-side Spacewave sync activity.
type UploadPhase int

const (
	// UploadPhaseIdle means no upload work is known.
	UploadPhaseIdle UploadPhase = iota
	// UploadPhaseDirtyPending means dirty blocks are waiting to upload.
	UploadPhaseDirtyPending
	// UploadPhasePushing means a packfile push is in flight.
	UploadPhasePushing
	// UploadPhaseError means upload sync has a recorded error.
	UploadPhaseError
	// UploadPhaseCorrupt means dirty blocks fail their hash on the local disk.
	// They stay queued and local, the rest of the queue uploads, and the store
	// publishes nothing while they stay: sync is held.
	UploadPhaseCorrupt
)

// Snapshot describes Spacewave cloud sync activity.
type Snapshot struct {
	// UploadPhase is the current aggregate upload phase.
	UploadPhase UploadPhase
	// PendingUploadBytes is the approximate dirty upload backlog in bytes.
	PendingUploadBytes int64
	// PendingUploadCount is the approximate count of dirty upload items.
	PendingUploadCount int
	// CorruptDirtyCount is the count of dirty blocks the last flush could not
	// upload because their local bytes fail their hash. They stay queued.
	CorruptDirtyCount int
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
	// MergeCount is the number of committed small-pack merges.
	MergeCount uint64
	// MergedPackCount is the number of packs replaced by committed merges.
	MergedPackCount uint64
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
	// UploadError is the latest push error of a store whose uploads are still
	// pending. It clears when that store's uploads succeed or drain.
	UploadError string
	// UploadHold is the account limit that refused the last push of a store
	// whose uploads are still pending. Its uploads wait for the account state
	// to change.
	UploadHold UploadHold
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
	// BlockStores contains source and convergence mechanics keyed by block store.
	BlockStores []BlockStoreSnapshot
}

// BuildSnapshot builds aggregate Spacewave sync telemetry from store states.
func BuildSnapshot(states []state) Snapshot {
	// Copy each store's counters and sum the totals.
	snap := Snapshot{
		StoreCount: len(states),
	}
	snap.BlockStores = make([]BlockStoreSnapshot, 0, len(states))
	for _, state := range states {
		snap.BlockStores = append(snap.BlockStores, BlockStoreSnapshot{
			BlockStoreID:             state.id,
			SharedObjectID:           state.sharedObjectID,
			DirectHitCount:           state.directHitCount,
			CloudHitCount:            state.cloudHitCount,
			CacheHitCount:            state.cacheHitCount,
			LastSource:               state.lastSource,
			AcceptedCheckpointHeight: state.acceptedCheckpointHeight,
			CloudRemoteSequence:      state.cloudRemoteSequence,
			CorruptDirtyCount:        state.corruptDirtyCount,
			HeldSince:                state.heldSince,
			HeldPartial:              state.heldPartial,
		})
		snap.PendingUploadBytes += state.pendingUploadBytes
		snap.PendingUploadCount += state.pendingUploadCount + state.linkedPendingCount + state.pendingPublications
		snap.CorruptDirtyCount += state.corruptDirtyCount
		snap.ActiveUploadBytes += state.activeUploadBytes
		snap.ActiveUploadTransferredBytes += state.activeUploadSent
		snap.InFlightPushes += state.inFlightPushes
		snap.PushCount += state.pushCount
		snap.PushedBytes += state.pushedBytes
		snap.DedupedUploadCount += state.dedupedUploadCount
		snap.DedupedUploadBytes += state.dedupedUploadBytes
		snap.MergeCount += state.mergeCount
		snap.MergedPackCount += state.mergedPackCount
		snap.PullActiveCount += state.pullActiveCount
		snap.LastPushAt = maxTime(snap.LastPushAt, state.lastPushAt)
		snap.LastPullAt = maxTime(snap.LastPullAt, state.lastPullAt)
		snap.LastActivityAt = maxTime(snap.LastActivityAt, state.lastActivityAt)
		if state.lastPushError != "" {
			snap.LastPushError = state.lastPushError
			snap.LastPushErrorAt = state.lastPushErrorAt
			pending := state.pendingUploadCount + state.linkedPendingCount + state.pendingPublications
			if pending != 0 || state.pendingUploadBytes != 0 || state.inFlightPushes != 0 {
				snap.UploadError = state.lastPushError
				if state.uploadHold != UploadHoldNone {
					snap.UploadHold = state.uploadHold
				}
			}
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
	slices.SortFunc(snap.BlockStores, func(a, b BlockStoreSnapshot) int {
		switch {
		case a.BlockStoreID < b.BlockStoreID:
			return -1
		case a.BlockStoreID > b.BlockStoreID:
			return 1
		default:
			return 0
		}
	})
	snap.UploadPhase = uploadPhase(snap)
	return snap
}

// uploadPhase names the upload state of snap, the most urgent first: a corrupt
// dirty block outranks a push error, since it fails every retry the same way.
func uploadPhase(snap Snapshot) UploadPhase {
	// A corrupt dirty block fails every retry: sync is held.
	if snap.CorruptDirtyCount > 0 {
		return UploadPhaseCorrupt
	}

	// Otherwise report the push and the queue.
	if snap.LastPushError != "" {
		return UploadPhaseError
	}
	if snap.InFlightPushes > 0 {
		return UploadPhasePushing
	}
	if snap.PendingUploadBytes > 0 || snap.PendingUploadCount > 0 {
		return UploadPhaseDirtyPending
	}
	return UploadPhaseIdle
}

func maxTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
