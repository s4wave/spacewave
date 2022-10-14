package hydra_api

import (
	"context"
	"time"

	volume "github.com/aperturerobotics/hydra/volume"
)

// ListVolumes lists basic volume information
func (a *API) ListVolumes(
	ctx context.Context,
	req *ListVolumesRequest,
) (*ListVolumesResponse, error) {
	var volumeInfos []*volume.VolumeInfo
	controllers := a.bus.GetControllers()
	for _, controller := range controllers {
		vc, ok := controller.(volume.Controller)
		if ok {
			subCtx, subCtxCancel := context.WithTimeout(ctx, time.Second*2)
			vol, err := vc.GetVolume(subCtx)
			subCtxCancel()
			if err != nil {
				continue
			}
			ci := vc.GetControllerInfo()
			volInfo, err := volume.NewVolumeInfo(ctx, ci, vol)
			if err != nil {
				continue
			}
			volumeInfos = append(volumeInfos, volInfo)
		}
	}

	return &ListVolumesResponse{
		Volumes: volumeInfos,
	}, nil
}
