package api_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/daemon/api"
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
			vol, err := vc.GetVolume(ctx)
			if err != nil {
				continue
			}
			volumeInfos = append(volumeInfos, vol.GetVolumeInfo())
		}
	}

	return &api.ListVolumesResponse{
		Volumes: volumeInfos,
	}, nil
}
