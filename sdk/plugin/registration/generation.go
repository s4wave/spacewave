package s4wave_plugin_registration

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// PrepareGeneration opens the private registration scope of this Go plugin's
// generation beneath the core root Resource. The plugin host supplies the
// family, the immutable manifest, and the installation binding, so instances of
// the plugin in several Spaces never collide on capability names.
//
// Registrations made through the returned scope stay hidden until Activate
// admits them together. Releasing the scope hides the whole generation. A
// historical worker publishes no current registrations and gets a nil scope.
func PrepareGeneration(
	ctx context.Context,
	b bus.Bus,
	core *resource_client.Client,
	root srpc.Client,
) (resource_client.ResourceRef, error) {
	// Read this worker's identity from its plugin host.
	serviceID := bldr_plugin.HostServiceIDPrefix + bldr_plugin.SRPCPluginHostServiceID
	hostClients, _, hostRef, err := bifrost_rpc.ExLookupRpcClient(ctx, b, serviceID, "", true, nil)
	if err != nil {
		return nil, errors.Wrap(err, "look up plugin host")
	}
	defer hostRef.Release()
	host := bldr_plugin.NewSRPCPluginHostClientWithServiceID(hostClients[0], serviceID)
	info, err := host.GetPluginInfo(ctx, &bldr_plugin.GetPluginInfoRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "get plugin info")
	}
	if info.GetHistorical() {
		return nil, nil
	}
	manifestRoot := info.GetManifestRef().GetManifestRef().GetRootRef().GetHash().MarshalString()
	if info.GetPluginId() == "" || manifestRoot == "" {
		return nil, errors.New("plugin has no immutable manifest")
	}

	// Open the generation scope under the caller's core Resource client.
	resp, err := NewSRPCRegistrationServiceClient(root).Prepare(ctx, &PrepareRequest{
		PluginId:     info.GetPluginId(),
		ManifestRoot: manifestRoot,
		InstanceKey:  info.GetInstanceKey(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "prepare registrations")
	}
	if resp.GetResourceId() == 0 {
		return nil, errors.New("plugin registration scope returned no resource id")
	}
	return core.CreateResourceReference(resp.GetResourceId()), nil
}

// Activate atomically replaces the plugin family's visible registrations with
// those made in scope.
func Activate(ctx context.Context, scope srpc.Client) error {
	_, err := NewSRPCGenerationServiceClient(scope).Activate(ctx, &ActivateRequest{})
	return err
}
