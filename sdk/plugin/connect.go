package s4wave_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// PluginResources holds the resource client and directive reference for a cross-plugin connection.
// Release must be called when done to clean up both the resource client and the plugin reference.
type PluginResources struct {
	// Client is the resource client for the target plugin.
	Client *resource_client.Client
	// pluginRef is the directive reference for the plugin load.
	pluginRef directive.Reference
}

// Release releases the resource client and the plugin directive reference.
func (p *PluginResources) Release() {
	if p.Client != nil {
		p.Client.Release()
	}
	if p.pluginRef != nil {
		p.pluginRef.Release()
	}
}

// ConnectPluginResources connects to another plugin's resource service.
// It waits for the target plugin to be loaded, then creates a resource client.
// The caller must call Release on the returned PluginResources when done.
func ConnectPluginResources(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
) (*PluginResources, error) {
	// Wait for the target plugin to be loaded and get its SRPC client.
	pluginClient, pluginRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
	if err != nil {
		return nil, err
	}

	// Create a ResourceService client from the plugin's SRPC client.
	resourceSvc := resource.NewSRPCResourceServiceClient(pluginClient)

	// Create the resource client (opens the persistent ResourceClient stream).
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		pluginRef.Release()
		return nil, err
	}

	return &PluginResources{
		Client:    resClient,
		pluginRef: pluginRef,
	}, nil
}
