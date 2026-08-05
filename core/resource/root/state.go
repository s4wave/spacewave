package resource_root

import (
	"context"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// StateAtomObjectStoreID is the object store ID for state atoms.
const StateAtomObjectStoreID = "state-atoms"

// newStateAtomManager creates a new state atom manager for the root server.
func newStateAtomManager(s *CoreRootServer) *resource_state.StateAtomManager {
	return resource_state.NewStateAtomManager(s.b, StateAtomObjectStoreID, bldr_plugin.PluginVolumeID)
}

// AccessStateAtom accesses a state atom resource.
func (s *CoreRootServer) AccessStateAtom(
	ctx context.Context,
	req *s4wave_root.AccessStateAtomRequest,
) (*s4wave_root.AccessStateAtomResponse, error) {
	// Acquire the caller resource context.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the requested state-atom store.
	storeID := req.GetStoreId()
	if storeID == "" {
		storeID = resource_state.DefaultStateAtomStoreID
	}

	// Open the store and publish its identifier.
	store, err := s.stateAtomMgr.GetOrCreateStore(ctx, storeID)
	if err != nil {
		return nil, err
	}
	s.trackStateAtomStoreID(ctx, storeID)

	// Register the state-atom resource.
	stateResource := resource_state.NewStateAtomResource(store)
	id, err := resourceCtx.AddResource(stateResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	// Return the registered resource identifier.
	return &s4wave_root.AccessStateAtomResponse{ResourceId: id}, nil
}
