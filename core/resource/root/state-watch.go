package resource_root

import (
	"context"

	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

func (s *CoreRootServer) getStateAtomStoreIndex(
	ctx context.Context,
) (*session.StateAtomStoreIndex, error) {
	// Serialize lazy index creation and reject closed servers.
	s.stateAtomStoreIndexMtx.Lock()
	defer s.stateAtomStoreIndexMtx.Unlock()
	if s.stateAtomStoreClosed {
		return nil, errors.New("root resource server is closed")
	}

	// Reuse the existing state-atom index when available.
	if s.stateAtomStoreIndex != nil {
		return s.stateAtomStoreIndex, nil
	}

	var (
		stateAtomStoreIndex *session.StateAtomStoreIndex
		release             func()
		err                 error
	)

	// Build the index through the test hook or plugin object store.
	if builder := s.stateAtomStoreIndexBuilder; builder != nil {
		stateAtomStoreIndex, release, err = builder(ctx)
	} else {
		objStoreHandle, _, diRef, buildErr := volume.ExBuildObjectStoreAPI(
			ctx,
			s.b,
			false,
			StateAtomObjectStoreID,
			bldr_plugin.PluginVolumeID,
			nil,
		)
		if buildErr != nil {
			return nil, buildErr
		}
		stateAtomStoreIndex = session.NewStateAtomStoreIndex(objStoreHandle.GetObjectStore())
		release = diRef.Release
	}

	// Publish the initialized index and release callback.
	if err != nil {
		return nil, err
	}

	s.stateAtomStoreIndex = stateAtomStoreIndex
	s.releaseStateAtomStoreIndex = release
	return s.stateAtomStoreIndex, nil
}

func (s *CoreRootServer) closeStateAtomStoreIndex() {
	s.stateAtomStoreIndexMtx.Lock()
	s.stateAtomStoreClosed = true
	release := s.releaseStateAtomStoreIndex
	s.stateAtomStoreIndex = nil
	s.releaseStateAtomStoreIndex = nil
	s.stateAtomStoreIndexMtx.Unlock()

	// Release the detached object-store index after unlocking.
	if release != nil {
		release()
	}
}

// WatchStateAtoms streams the known root state atom store ids on change.
func (s *CoreRootServer) WatchStateAtoms(
	_ *s4wave_root.WatchStateAtomsRequest,
	strm s4wave_root.SRPCRootResourceService_WatchStateAtomsStream,
) error {
	stateAtomStoreIndex, err := s.getStateAtomStoreIndex(strm.Context())
	if err != nil {
		return err
	}

	return stateAtomStoreIndex.WatchStoreIDs(
		strm.Context(),
		func(storeIDs []string) error {
			return strm.Send(&s4wave_root.WatchStateAtomsResponse{
				StoreIds:   storeIDs,
				StoreCount: uint32(len(storeIDs)),
			})
		},
	)
}

func (s *CoreRootServer) trackStateAtomStoreID(ctx context.Context, storeID string) {
	stateAtomStoreIndex, err := s.getStateAtomStoreIndex(ctx)
	if err != nil {
		s.le.WithError(errors.Wrap(err, "build state atom store index")).Debug(
			"failed to track root state atom store id",
		)
		return
	}
	stateAtomStoreIndex.TrackStoreID(storeID)
}
