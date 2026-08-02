package synctelemetry

import (
	"slices"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
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
)

// BlockSource identifies the observed source of a block read.
type BlockSource uint8

const (
	// BlockSourceUnknown means no source has been observed.
	BlockSourceUnknown BlockSource = iota
	// BlockSourceCache means the local upper cache satisfied the read.
	BlockSourceCache
	// BlockSourceDirect means demand-driven DEX satisfied the read.
	BlockSourceDirect
	// BlockSourceCloud means the Cloud packfile baseline satisfied the read.
	BlockSourceCloud
)

// BlockStoreSnapshot describes source and convergence mechanics for one block store.
type BlockStoreSnapshot struct {
	BlockStoreID              string
	SharedObjectID            string
	DirectHitCount            uint64
	CloudHitCount             uint64
	CacheHitCount             uint64
	LastSource                BlockSource
	AcceptedRootInnerSequence uint64
	CloudRemoteSequence       uint64
}

// Snapshot describes Spacewave cloud sync activity.
type Snapshot struct {
	// UploadPhase is the current aggregate upload phase.
	UploadPhase UploadPhase
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
	// BlockStores contains source and convergence mechanics keyed by block store.
	BlockStores []BlockStoreSnapshot
}

// FetchStatsProvider returns packfile fetch-side telemetry.
type FetchStatsProvider interface {
	SnapshotStats() packfile_store.PackfileStoreStats
}

// StatsChangedProvider notifies the telemetry Store when fetch-side stats change.
type StatsChangedProvider interface {
	SetStatsChangedCallback(func())
}

// Store owns sync telemetry state keyed by block store id.
type Store struct {
	bcast  broadcast.Broadcast
	states map[string]*state
}

type state struct {
	id         string
	fetchStats FetchStatsProvider

	pendingUploadBytes        int64
	pendingUploadCount        int
	activeUploadBytes         int64
	activeUploadSent          int64
	inFlightPushes            int
	pushCount                 uint64
	pushedBytes               int64
	dedupedUploadCount        uint64
	dedupedUploadBytes        int64
	pullActiveCount           int
	lastPushAt                time.Time
	lastPullAt                time.Time
	lastActivityAt            time.Time
	lastPushError             string
	lastPushErrorAt           time.Time
	lastPullError             string
	lastPullErrorAt           time.Time
	directHitCount            uint64
	cloudHitCount             uint64
	cacheHitCount             uint64
	lastSource                BlockSource
	acceptedRootInnerSequence uint64
	sharedObjectID            string
	cloudRemoteSequence       uint64
}

// Broadcast returns the broadcast guarding Spacewave sync telemetry.
func (s *Store) Broadcast() *broadcast.Broadcast {
	return &s.bcast
}

// Snapshot returns aggregate Spacewave sync telemetry.
func (s *Store) Snapshot() Snapshot {
	states := s.cloneStates()
	return BuildSnapshot(states)
}

// SnapshotWithWait returns aggregate sync telemetry and its wait channel.
func (s *Store) SnapshotWithWait() (Snapshot, <-chan struct{}) {
	var ch <-chan struct{}
	var states []state
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		states = s.cloneStatesLocked()
	})
	return BuildSnapshot(states), ch
}

// RegisterStore registers fetch-side stats for a block store.
func (s *Store) RegisterStore(bstoreID string, fetchStats FetchStatsProvider) func() {
	if bstoreID == "" {
		return func() {}
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.states == nil {
			s.states = make(map[string]*state)
		}
		entry := s.states[bstoreID]
		if entry == nil {
			entry = &state{id: bstoreID}
			s.states[bstoreID] = entry
		}
		entry.fetchStats = fetchStats
		broadcast()
	})
	if notifier, ok := fetchStats.(StatsChangedProvider); ok {
		notifier.SetStatsChangedCallback(s.broadcast)
	}
	return func() {
		if notifier, ok := fetchStats.(StatsChangedProvider); ok {
			notifier.SetStatsChangedCallback(nil)
		}
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if s.states == nil {
				return
			}
			delete(s.states, bstoreID)
			broadcast()
		})
	}
}

// SetPending replaces the pending upload backlog for a block store.
func (s *Store) SetPending(bstoreID string, bytes int64, count int) {
	if bytes < 0 {
		bytes = 0
	}
	if count < 0 {
		count = 0
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.pendingUploadBytes = bytes
		state.pendingUploadCount = count
		broadcast()
	})
}

// AddDirty records newly dirty upload bytes.
func (s *Store) AddDirty(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.pendingUploadBytes += bytes
		state.pendingUploadCount++
		state.lastActivityAt = now
		broadcast()
	})
}

// StartPush records a started packfile push.
func (s *Store) StartPush(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.inFlightPushes++
		state.activeUploadBytes += bytes
		broadcast()
	})
}

// SetPushProgress records in-flight upload progress.
func (s *Store) SetPushProgress(bstoreID string, bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
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

// FinishPush records a completed packfile push.
func (s *Store) FinishPush(bstoreID string, bytes int64, err error) {
	if bytes < 0 {
		bytes = 0
	}
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
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

// RecordError records a sync error that happened outside an active push/pull.
func (s *Store) RecordError(bstoreID string, err error) {
	if err == nil {
		return
	}
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.lastPushError = err.Error()
		state.lastPushErrorAt = now
		state.lastActivityAt = now
		broadcast()
	})
}

// AddDeduped records dirty blocks skipped because they already exist remotely.
func (s *Store) AddDeduped(bstoreID string, bytes int64, count int) {
	if bytes < 0 {
		bytes = 0
	}
	if count < 0 {
		count = 0
	}
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.dedupedUploadBytes += bytes
		state.dedupedUploadCount += uint64(count)
		state.lastActivityAt = now
		broadcast()
	})
}

// StartPull records a started sync pull.
func (s *Store) StartPull(bstoreID string) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.pullActiveCount++
		broadcast()
	})
}

// FinishPull records a completed sync pull.
func (s *Store) FinishPull(bstoreID string, err error) {
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
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

// RecordBlockSource records the source selected for a completed block read.
func (s *Store) RecordBlockSource(bstoreID string, source BlockSource) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		switch source {
		case BlockSourceCache:
			state.cacheHitCount++
		case BlockSourceDirect:
			state.directHitCount++
		case BlockSourceCloud:
			state.cloudHitCount++
		default:
			return
		}
		state.lastSource = source
		broadcast()
	})
}

// SetAcceptedRoot records the latest accepted SharedObject root for a block store.
func (s *Store) SetAcceptedRoot(bstoreID, sharedObjectID string, sequence uint64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		if sequence == state.acceptedRootInnerSequence && sharedObjectID == state.sharedObjectID {
			return
		}
		state.acceptedRootInnerSequence = sequence
		state.sharedObjectID = sharedObjectID
		broadcast()
	})
}

// SetCloudRemoteSequence records the latest observed Cloud block-store sequence.
func (s *Store) SetCloudRemoteSequence(bstoreID string, sequence uint64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		if sequence <= state.cloudRemoteSequence {
			return
		}
		state.cloudRemoteSequence = sequence
		broadcast()
	})
}

// BuildSnapshot builds aggregate Spacewave sync telemetry from store states.
func BuildSnapshot(states []state) Snapshot {
	snap := Snapshot{
		StoreCount: len(states),
	}
	snap.BlockStores = make([]BlockStoreSnapshot, 0, len(states))
	for _, state := range states {
		snap.BlockStores = append(snap.BlockStores, BlockStoreSnapshot{
			BlockStoreID:              state.id,
			SharedObjectID:            state.sharedObjectID,
			DirectHitCount:            state.directHitCount,
			CloudHitCount:             state.cloudHitCount,
			CacheHitCount:             state.cacheHitCount,
			LastSource:                state.lastSource,
			AcceptedRootInnerSequence: state.acceptedRootInnerSequence,
			CloudRemoteSequence:       state.cloudRemoteSequence,
		})
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

func (s *Store) broadcast() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

func (s *Store) cloneStates() []state {
	var states []state
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		states = s.cloneStatesLocked()
	})
	return states
}

func (s *Store) cloneStatesLocked() []state {
	if len(s.states) == 0 {
		return nil
	}
	states := make([]state, 0, len(s.states))
	for _, state := range s.states {
		if state == nil {
			continue
		}
		states = append(states, *state)
	}
	return states
}

func (s *Store) getOrCreateStateLocked(bstoreID string) *state {
	if s.states == nil {
		s.states = make(map[string]*state)
	}
	entry := s.states[bstoreID]
	if entry == nil {
		entry = &state{id: bstoreID}
		s.states[bstoreID] = entry
	}
	return entry
}

func uploadPhase(snap Snapshot) UploadPhase {
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

// _ is a type assertion
var _ FetchStatsProvider = ((*packfile_store.PackfileStore)(nil))
