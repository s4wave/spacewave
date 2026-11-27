package s4wave_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// PluginResources holds the resource client and directive reference for a cross-plugin connection.
// Release must be called when done to clean up both the resource client and the plugin reference.
type PluginResources struct {
	// client is the resource client for the target plugin.
	client *resource_client.Client
	// pluginRef is the directive reference for the plugin load.
	pluginRef directive.Reference
}

// Release releases the resource client and the plugin directive reference.
func (p *PluginResources) Release() {
	if p.client != nil {
		p.client.Release()
	}
	if p.pluginRef != nil {
		p.pluginRef.Release()
	}
}

// AccessRootResource acquires a reference to the connected plugin's root.
// The caller releases the reference before releasing the connection.
func (p *PluginResources) AccessRootResource() resource_client.ResourceRef {
	return p.client.AccessRootResource()
}

// CreateResourceReference adopts a resource returned by the connected plugin.
// The caller releases the reference when done with the resource.
func (p *PluginResources) CreateResourceReference(resourceID uint32) resource_client.ResourceRef {
	return p.client.CreateResourceReference(resourceID)
}

// OpenRootClient acquires the plugin root for RPC calls.
// The caller releases the returned reference before releasing the connection.
func (p *PluginResources) OpenRootClient() (srpc.Client, resource_client.ResourceRef, error) {
	ref := p.client.AccessRootResource()
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, nil, err
	}
	return client, ref, nil
}

// AttachResource lends a local resource to the connected plugin.
// The caller detaches the returned ID at the end of the capability's lifetime.
func (p *PluginResources) AttachResource(ctx context.Context, label string, mux srpc.Invoker) (uint32, error) {
	return p.client.AttachResource(ctx, label, mux)
}

// DetachResource withdraws a resource lent to the connected plugin.
func (p *PluginResources) DetachResource(ctx context.Context, resourceID uint32) error {
	return p.client.DetachResource(ctx, resourceID)
}

// Retired reports whether the connected resource generation has drained.
func (p *PluginResources) Retired() bool {
	return p.client.Retired()
}

// _ verifies that PluginResources supplies resource adoption without exposing its client.
var _ resource_client.ReferenceCreator = (*PluginResources)(nil)
