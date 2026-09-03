package inmem

import (
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
)

type scopeKey struct {
	volumeID      string
	objectStoreID string
	key           string
}

func newScopeKey(scope coord.Scope) scopeKey {
	return scopeKey{
		volumeID:      scope.VolumeID,
		objectStoreID: scope.ObjectStoreID,
		key:           scope.Key,
	}
}

type scopeState struct {
	generation uint64
	root       *bucket.ObjectRef
	// lastKeyPrefix is the key prefix invalidated by the most recent publish
	// that carried one. Replayed events carry it so a watcher that missed the
	// live publish still observes the outstanding invalidation.
	lastKeyPrefix []byte
	locked        bool
	watchers      map[uint64]*watch
}

func newScopeState() *scopeState {
	return &scopeState{
		watchers: make(map[uint64]*watch),
	}
}

func (s *scopeState) snapshot(scope coord.Scope) *coord.Snapshot {
	return &coord.Snapshot{
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
		Generation:    s.generation,
		Root:          s.root.Clone(),
	}
}

func (s *scopeState) snapshotEvent(scope coord.Scope) coord.Event {
	return coord.Event{
		VolumeID:         scope.VolumeID,
		ObjectStoreID:    scope.ObjectStoreID,
		Generation:       s.generation,
		RootChanged:      s.root.Clone(),
		KeyPrefixChanged: append([]byte(nil), s.lastKeyPrefix...),
	}
}

func (s *scopeState) publishLocked(event coord.Event) {
	event.RootChanged = event.RootChanged.Clone()
	if event.KeyPrefixChanged != nil {
		event.KeyPrefixChanged = append([]byte(nil), event.KeyPrefixChanged...)
		s.lastKeyPrefix = append([]byte(nil), event.KeyPrefixChanged...)
	}
	for _, watch := range s.watchers {
		watch.sendLocked(event)
	}
}
