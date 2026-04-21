//go:build !js

package coord

import (
	"context"
	"slices"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/util/broadcast"
)

// ParticipantWatcher polls the participant registry keyed off commitCounter
// changes and maintains an in-memory snapshot of active participants.
type ParticipantWatcher struct {
	db    *bdb.DB
	bcast broadcast.Broadcast

	// snapshot is the latest known set of participants, guarded by bcast.
	snapshot []*ParticipantRecord
}

// NewParticipantWatcher creates a new watcher.
func NewParticipantWatcher(db *bdb.DB) *ParticipantWatcher {
	return &ParticipantWatcher{db: db}
}

// GetParticipants returns the current participant snapshot.
// Must be called inside bcast.HoldLock.
func (w *ParticipantWatcher) GetParticipants() []*ParticipantRecord {
	return w.snapshot
}

// Run polls the participant registry using commitCounter for wake-up.
// Blocks until ctx is cancelled.
func (w *ParticipantWatcher) Run(ctx context.Context) error {
	var lastCounter uint64
	for {
		counter, err := w.db.WaitCommitCounter(ctx, lastCounter)
		if err != nil {
			return err
		}
		lastCounter = counter

		var records []*ParticipantRecord
		err = w.db.View(func(tx *bdb.Tx) error {
			var readErr error
			records, readErr = ListParticipants(tx)
			return readErr
		})
		if err != nil {
			return err
		}

		// Only broadcast if the participant set actually changed.
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if !participantsEqual(w.snapshot, records) {
				w.snapshot = records
				broadcast()
			}
		})
	}
}

// participantsEqual returns true if both slices contain the same
// participants with the same PIDs and roles (ignoring heartbeat timestamps).
func participantsEqual(a, b []*ParticipantRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GetPid() != b[i].GetPid() ||
			a[i].GetRole() != b[i].GetRole() ||
			a[i].GetSrpcSocketPath() != b[i].GetSrpcSocketPath() {
			return false
		}
	}
	return true
}

// WaitParticipants waits for the participant list to satisfy the given
// predicate. Returns the matching snapshot.
func (w *ParticipantWatcher) WaitParticipants(ctx context.Context, match func([]*ParticipantRecord) bool) ([]*ParticipantRecord, error) {
	var result []*ParticipantRecord
	err := w.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		snap := w.snapshot
		if match(snap) {
			result = slices.Clone(snap)
			return true, nil
		}
		return false, nil
	})
	return result, err
}
