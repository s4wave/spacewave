package resource_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// WatchProcessBindings streams decisions independently of the Space runtime.
func (r *SpaceResource) WatchProcessBindings(
	_ *s4wave_space.WatchProcessBindingsRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchProcessBindingsStream,
) error {
	// Stream changed process decisions while the mounted Space is available.
	if r.bindingRegistry == nil {
		return errors.New("process binding watch unavailable")
	}
	ctx := strm.Context()
	spaceID := space.SpaceEngineId(r.space.GetSharedObjectRef())
	var previous *s4wave_space.WatchProcessBindingsResponse
	for {
		changed := r.bindingRegistry.Changed()
		bindings, err := listProcessBindingInfos(ctx, r.b, bldr_plugin.PluginVolumeID, process_binding.DefaultObjectStoreID, spaceID)
		if err != nil {
			return err
		}
		response := &s4wave_space.WatchProcessBindingsResponse{ProcessBindings: bindings}
		if previous == nil || !response.EqualVT(previous) {
			if err := strm.Send(response); err != nil {
				return err
			}
			previous = response
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// listProcessBindingInfos reads local decisions from the Space's account store.
func listProcessBindingInfos(ctx context.Context, b bus.Bus, volumeID, storeID, spaceID string) ([]*s4wave_space.ProcessBindingInfo, error) {
	// Read and project local process decisions from the Space account store.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, b, true, storeID, volumeID, nil)
	if err != nil {
		return nil, err
	}
	defer ref.Release()
	bindings, err := process_binding.ListProcessBindings(ctx, handle.GetObjectStore(), spaceID)
	if err != nil {
		return nil, err
	}
	infos := make([]*s4wave_space.ProcessBindingInfo, 0, len(bindings))
	for _, binding := range bindings {
		infos = append(infos, &s4wave_space.ProcessBindingInfo{
			ObjectKey: binding.GetObjectKey(),
			TypeId:    binding.GetTypeId(),
			Approved:  binding.GetState() == s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
			DecidedAt: binding.GetDecidedAt(),
		})
	}
	return infos, nil
}
