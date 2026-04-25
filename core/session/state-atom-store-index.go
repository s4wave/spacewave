package session

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/object"
)

const stateAtomStoreKeyPrefix = "state/"

// StateAtomStoreIndex tracks known state atom store ids for an object store.
type StateAtomStoreIndex struct {
	// objStore is the backing object store.
	objStore object.ObjectStore

	// bcast guards trackedStoreIDs.
	bcast broadcast.Broadcast
	// trackedStoreIDs contains store ids observed through AccessStateAtomStore.
	trackedStoreIDs map[string]struct{}
}

// NewStateAtomStoreIndex creates a new StateAtomStoreIndex.
func NewStateAtomStoreIndex(objStore object.ObjectStore) *StateAtomStoreIndex {
	return &StateAtomStoreIndex{
		objStore:        objStore,
		trackedStoreIDs: make(map[string]struct{}),
	}
}

// TrackStoreID records a known state atom store id and broadcasts on additions.
func (s *StateAtomStoreIndex) TrackStoreID(storeID string) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if _, ok := s.trackedStoreIDs[storeID]; ok {
			return
		}
		s.trackedStoreIDs[storeID] = struct{}{}
		broadcast()
	})
}

// SnapshotStoreIDs returns the known state atom store ids.
func (s *StateAtomStoreIndex) SnapshotStoreIDs(ctx context.Context) ([]string, error) {
	trackedStoreIDs, _ := s.snapshotTrackedStoreIDs()
	return s.buildStoreIDsSnapshot(ctx, trackedStoreIDs)
}

// WatchStoreIDs watches the known state atom store ids for changes.
func (s *StateAtomStoreIndex) WatchStoreIDs(
	ctx context.Context,
	cb func(storeIDs []string) error,
) error {
	var prevStoreIDs []string
	for {
		trackedStoreIDs, waitCh := s.snapshotTrackedStoreIDs()
		storeIDs, err := s.buildStoreIDsSnapshot(ctx, trackedStoreIDs)
		if err != nil {
			return err
		}
		if !slices.Equal(prevStoreIDs, storeIDs) {
			if err := cb(storeIDs); err != nil {
				return err
			}
			prevStoreIDs = storeIDs
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

func (s *StateAtomStoreIndex) snapshotTrackedStoreIDs() (map[string]struct{}, <-chan struct{}) {
	var trackedStoreIDs map[string]struct{}
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		trackedStoreIDs = maps.Clone(s.trackedStoreIDs)
		waitCh = getWaitCh()
	})
	return trackedStoreIDs, waitCh
}

func (s *StateAtomStoreIndex) buildStoreIDsSnapshot(
	ctx context.Context,
	trackedStoreIDs map[string]struct{},
) ([]string, error) {
	otx, err := s.objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	err = otx.ScanPrefixKeys(ctx, []byte(stateAtomStoreKeyPrefix), func(key []byte) error {
		keyStr := string(key)
		if !strings.HasPrefix(keyStr, stateAtomStoreKeyPrefix) {
			return errors.Errorf("unexpected state atom key prefix: %q", keyStr)
		}
		trackedStoreIDs[strings.TrimPrefix(keyStr, stateAtomStoreKeyPrefix)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	storeIDs := make([]string, 0, len(trackedStoreIDs))
	for storeID := range trackedStoreIDs {
		storeIDs = append(storeIDs, storeID)
	}
	sort.Strings(storeIDs)
	return storeIDs, nil
}
