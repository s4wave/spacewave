package hydra_api_controller

import (
	"context"
	"time"

	api "github.com/aperturerobotics/hydra/daemon/api"
	volume "github.com/aperturerobotics/hydra/volume"
)

// ListVolumes lists basic volume information
func (a *API) ListVolumes(
	ctx context.Context,
	req *api.ListVolumesRequest,
) (*api.ListVolumesResponse, error) {
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
			volInfo, err := volume.NewVolumeInfo(ci, vol)
			if err != nil {
				continue
			}
			volumeInfos = append(volumeInfos, volInfo)
		}
	}

	return &api.ListVolumesResponse{
		Volumes: volumeInfos,
	}, nil
}
