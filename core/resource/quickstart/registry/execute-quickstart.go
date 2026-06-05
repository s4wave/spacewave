package resource_quickstart_registry

import (
	"context"

	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// ExecuteQuickstart runs a registered Quickstart seed handler against a mounted Space.
func (r *QuickstartRegistryResource) ExecuteQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.ExecuteQuickstartRequest,
) (*s4wave_quickstart_registry.ExecuteQuickstartResponse, error) {
	quickstartID := req.GetQuickstartId()
	if quickstartID == "" {
		return nil, ErrQuickstartIdRequired
	}
	if req.GetSpaceResourceId() == 0 {
		return nil, ErrSpaceResourceIdRequired
	}
	reg := r.LookupRegistration(quickstartID)
	if reg == nil {
		return nil, ErrQuickstartNotRegistered
	}
	if r.b == nil {
		return nil, ErrQuickstartExecutionUnavailable
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	spaceValue, err := resourceCtx.GetResourceValue(req.GetSpaceResourceId())
	if err != nil {
		return nil, err
	}
	spaceResource, ok := spaceValue.(*resource_space.SpaceResource)
	if !ok || spaceResource == nil {
		return nil, ErrSpaceResourceRequired
	}

	resourceClientCtx := resourceCtx.Context()
	resources, err := s4wave_plugin.ConnectPluginResources(resourceClientCtx, r.b, reg.GetPluginId())
	if err != nil {
		return nil, errors.Wrap(err, "connect to quickstart plugin")
	}
	defer resources.Release()

	lookupOp := space_world_optypes.BuildSpaceLookupOp(r.b, r.le, spaceResource.GetWorldEngineID())
	engineInfo := &s4wave_world.EngineInfo{
		EngineId: spaceResource.GetWorldEngineID(),
		BucketId: spaceResource.GetWorldEngineBucketID(),
	}
	engineResource := resource_world.NewEngineResource(
		r.le,
		r.b,
		spaceResource.GetWorldEngine(),
		lookupOp,
		engineInfo,
	)
	engineResourceID, err := resources.Client.AttachResource(ctx, "quickstart-world-engine", engineResource.GetMux())
	if err != nil {
		return nil, errors.Wrap(err, "attach quickstart world engine")
	}
	defer func() {
		_ = resources.Client.DetachResource(ctx, engineResourceID)
	}()

	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return nil, errors.Wrap(err, "get plugin root client")
	}
	defer rootRef.Release()

	handler := s4wave_quickstart_registry.NewSRPCQuickstartHandlerServiceClient(rootClient)
	resp, err := handler.SeedQuickstart(ctx, &s4wave_quickstart_registry.SeedQuickstartRequest{
		QuickstartId:             quickstartID,
		AttachedEngineResourceId: engineResourceID,
	})
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"seed quickstart plugin_id=%s capability=engine attached_root_id=%d path=SeedQuickstart quickstart_id=%s",
			reg.GetPluginId(),
			engineResourceID,
			quickstartID,
		)
	}

	return &s4wave_quickstart_registry.ExecuteQuickstartResponse{
		IndexPath: resp.GetIndexPath(),
		PluginIds: mergePluginIDs(
			reg.GetRequiredPluginIds(),
			resp.GetPluginIds(),
		),
	}, nil
}
