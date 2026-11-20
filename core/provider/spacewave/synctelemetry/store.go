package synctelemetry

import (
	"time"

	"github.com/aperturerobotics/util/broadcast"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// FetchStatsProvider returns packfile fetch-side telemetry.
type FetchStatsProvider interface {
	// SnapshotStats returns the current fetch-side counters.
	SnapshotStats() packfile_store.PackfileStoreStats
}

// StatsChangedProvider notifies the telemetry Store when fetch-side stats change.
type StatsChangedProvider interface {
	// SetStatsChangedCallback sets the function called after the stats
	// change, or clears it when nil.
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

	pendingUploadBytes       int64
	pendingUploadCount       int
	linkedPendingCount       int
	corruptDirtyCount        int
	heldSince                time.Time
	heldPartial              bool
	pendingPublications      int
	activeUploadBytes        int64
	activeUploadSent         int64
	inFlightPushes           int
	pushCount                uint64
	pushedBytes              int64
	dedupedUploadCount       uint64
	dedupedUploadBytes       int64
	mergeCount               uint64
	mergedPackCount          uint64
	pullActiveCount          int
	lastPushAt               time.Time
	lastPullAt               time.Time
	lastActivityAt           time.Time
	lastPushError            string
	lastPushErrorAt          time.Time
	uploadHold               UploadHold
	lastPullError            string
	lastPullErrorAt          time.Time
	directHitCount           uint64
	cloudHitCount            uint64
	cacheHitCount            uint64
	lastSource               BlockSource
	acceptedCheckpointHeight uint64
	sharedObjectID           string
	cloudRemoteSequence      uint64
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
		// Find or add the store's entry and attach its fetch stats.
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

// SetLinkedPending replaces the count of linked file chunks queued for upload
// beside the dirty backlog.
func (s *Store) SetLinkedPending(bstoreID string, count int) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.getOrCreateStateLocked(bstoreID).linkedPendingCount = max(count, 0)
		broadcast()
	})
}

// SetCorruptDirty records the count of dirty blocks the last flush held back
// because their local bytes fail their hash, since when the store holds them,
// and whether the count is a lower bound because the flush stopped early.
// While count is not zero the store publishes nothing.
func (s *Store) SetCorruptDirty(bstoreID string, count int, since time.Time, partial bool) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state := s.getOrCreateStateLocked(bstoreID)
		state.corruptDirtyCount, state.heldSince = max(count, 0), since
		state.heldPartial = partial && count > 0
		broadcast()
	})
}

// SetPendingPublications keeps metadata awaiting cloud acknowledgment visible
// after all payload blocks have uploaded.
func (s *Store) SetPendingPublications(bstoreID string, count int) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.getOrCreateStateLocked(bstoreID).pendingPublications = max(count, 0)
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
		// Clamp the progress to the active push and skip an unchanged value.
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
		// Take the push out of the in-flight totals.
		state := s.getOrCreateStateLocked(bstoreID)
		if state.inFlightPushes > 0 {
			state.inFlightPushes--
		}
		state.activeUploadBytes = max(state.activeUploadBytes-bytes, 0)
		if state.activeUploadBytes == 0 {
			state.activeUploadSent = 0
		}

		// Record the result: an error, or a push that clears the last one.
		state.uploadHold = uploadHoldOf(err)
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

// RecordError records a sync error outside an active push/pull. A successful
// checkpoint clears the previous error without counting another payload push.
func (s *Store) RecordError(bstoreID string, err error) {
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace the last error, clearing it on success.
		state := s.getOrCreateStateLocked(bstoreID)
		state.lastPushError = ""
		state.lastPushErrorAt = time.Time{}
		state.uploadHold = uploadHoldOf(err)
		if err != nil {
			state.lastPushError = err.Error()
			state.lastPushErrorAt = now
		}
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
		// Add the skipped blocks to the dedupe totals.
		state := s.getOrCreateStateLocked(bstoreID)
		state.dedupedUploadBytes += bytes
		state.dedupedUploadCount += uint64(count) //nolint:gosec // count is clamped to a non-negative in-memory event count above.
		state.lastActivityAt = now
		broadcast()
	})
}

// AddMerge records one committed merge that replaced mergedPacks packs.
func (s *Store) AddMerge(bstoreID string, mergedPacks int) {
	now := time.Now()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Add the merge to the merge totals.
		state := s.getOrCreateStateLocked(bstoreID)
		state.mergeCount++
		state.mergedPackCount += uint64(max(mergedPacks, 0)) //nolint:gosec // clamped to a non-negative count.
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
		// End the pull and record its result.
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

// SetAcceptedCheckpoint records the height of the latest accepted SharedObject checkpoint for a block store.
func (s *Store) SetAcceptedCheckpoint(bstoreID, sharedObjectID string, height uint64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Record a changed height and broadcast it.
		state := s.getOrCreateStateLocked(bstoreID)
		if height == state.acceptedCheckpointHeight && sharedObjectID == state.sharedObjectID {
			return
		}
		state.acceptedCheckpointHeight = height
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

// _ is a type assertion
var _ FetchStatsProvider = (*packfile_store.PackfileStore)(nil)
