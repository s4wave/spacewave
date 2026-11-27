package s4wave_plugin_registration

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
)

// corePluginID is the plugin that serves the capability registries.
const corePluginID = "spacewave-core"

// RegisterFunc makes a plugin's registrations through scope and returns the
// references that hold them, including those made before an error. client
// owns every returned reference.
type RegisterFunc func(
	ctx context.Context,
	client resource_client.ReferenceCreator,
	scope srpc.Client,
) ([]resource_client.ResourceRef, error)

// Holder publishes a Go plugin's capabilities on the core plugin. A startup
// controller embeds a Holder and calls Hold from Execute, so the plugin reports
// ready only after its first registration pass settles.
type Holder struct {
	// register makes the plugin's registrations.
	register RegisterFunc
	// settled is closed once the first registration pass settles.
	settled chan struct{}
	// settleOnce closes settled.
	settleOnce sync.Once
}

// NewHolder constructs a Holder that registers with register.
func NewHolder(register RegisterFunc) *Holder {
	return &Holder{register: register, settled: make(chan struct{})}
}

// WaitInitialCapabilities returns once the first registration pass has
// admitted the registrations, found a historical worker, or failed.
func (h *Holder) WaitInitialCapabilities(ctx context.Context) error {
	select {
	case <-h.settled:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// settle records that the first registration pass has settled.
func (h *Holder) settle() {
	h.settleOnce.Do(func() {
		close(h.settled)
	})
}

// Hold publishes this Go plugin's capabilities on the core plugin until ctx
// ends. It registers in the plugin's private generation, then admits the
// registrations together, so a Space plugin's capabilities route to its own
// installation. A historical worker registers nothing and waits for ctx.
func (h *Holder) Hold(ctx context.Context, b bus.Bus) error {
	// A pass that fails settles too: callers then see the missing capability.
	defer h.settle()

	// Connect to the core plugin's root Resource.
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, b, corePluginID)
	if err != nil {
		return err
	}
	defer resources.Release()
	root, rootRef, err := resources.OpenRootClient()
	if err != nil {
		return err
	}
	defer rootRef.Release()

	// Open the generation scope. A historical worker gets none.
	scopeRef, err := PrepareGeneration(ctx, b, resources, root)
	if err != nil {
		return err
	}
	if scopeRef == nil {
		h.settle()
		<-ctx.Done()
		return nil
	}
	defer scopeRef.Release()
	scope, err := scopeRef.GetClient()
	if err != nil {
		return err
	}

	// Register through the scope, then admit the registrations together.
	refs, err := h.register(ctx, resources, scope)
	defer func() {
		for _, ref := range slices.Backward(refs) {
			ref.Release()
		}
	}()
	if err != nil {
		return err
	}
	if err := Activate(ctx, scope); err != nil {
		return errors.Wrap(err, "activate registrations")
	}
	h.settle()
	<-ctx.Done()
	return nil
}

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
	core resource_client.ReferenceCreator,
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

	// Require the plugin identity and immutable manifest root.
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

	// Wrap the returned scope resource for the caller.
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
