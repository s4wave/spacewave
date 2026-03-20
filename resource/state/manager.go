package resource_state

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/object"
	"github.com/aperturerobotics/hydra/volume"
)

// DefaultStateAtomStoreID is the default store ID for the global state atom.
const DefaultStateAtomStoreID = "ui-state"

// StateAtomManager manages state atom stores with lazy initialization.
type StateAtomManager struct {
	mtx             sync.Mutex
	b               bus.Bus
	objStore        object.ObjectStore
	releaseObjStore func()
	stores          map[string]*ObjectStoreStateAtom
	objectStoreID   string
	volumeID        string
}

// NewStateAtomManager creates a new StateAtomManager.
// objectStoreID is the ID for the object store used to persist state atoms.
// volumeID is the volume ID where the object store will be created.
func NewStateAtomManager(b bus.Bus, objectStoreID, volumeID string) *StateAtomManager {
	return &StateAtomManager{
		b:             b,
		stores:        make(map[string]*ObjectStoreStateAtom),
		objectStoreID: objectStoreID,
		volumeID:      volumeID,
	}
}

// GetOrCreateStore gets or creates a state atom store by ID.
func (m *StateAtomManager) GetOrCreateStore(ctx context.Context, storeID string) (StateAtomStore, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Check if store already exists
	if store, ok := m.stores[storeID]; ok {
		return store, nil
	}

	// Lazily create object store
	if m.objStore == nil {
		objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
			ctx,
			m.b,
			false,
			m.objectStoreID,
			m.volumeID,
			nil,
		)
		if err != nil {
			return nil, err
		}
		m.objStore = objStoreHandle.GetObjectStore()
		m.releaseObjStore = diRef.Release
	}

	// Create new store
	store := NewObjectStoreStateAtom(storeID, m.objStore)
	m.stores[storeID] = store
	return store, nil
}

// Release releases all resources held by the manager.
func (m *StateAtomManager) Release() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.releaseObjStore != nil {
		m.releaseObjStore()
		m.releaseObjStore = nil
	}
}
