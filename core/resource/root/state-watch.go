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
	s.stateAtomStoreIndexMtx.Lock()
	defer s.stateAtomStoreIndexMtx.Unlock()

	if s.stateAtomStoreIndex != nil {
		return s.stateAtomStoreIndex, nil
	}

	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		s.b,
		false,
		StateAtomObjectStoreID,
		bldr_plugin.PluginVolumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}

	s.stateAtomStoreIndex = session.NewStateAtomStoreIndex(objStoreHandle.GetObjectStore())
	s.releaseStateAtomStoreIndex = diRef.Release
	return s.stateAtomStoreIndex, nil
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
