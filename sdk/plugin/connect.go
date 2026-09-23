package s4wave_plugin

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

const pluginResourceConnectAttempts = 3

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
	return connectPluginResources(ctx, b, pluginID, "")
}

// ConnectPluginResourcesAtManifest connects to an exact retained executable.
// The caller releases both the resource connection and immutable plugin reference.
func ConnectPluginResourcesAtManifest(ctx context.Context, b bus.Bus, pluginID, manifestRoot string) (*PluginResources, error) {
	return connectPluginResources(ctx, b, pluginID, manifestRoot)
}

// connectPluginResources acquires a client within the selected executable's lifetime.
func connectPluginResources(ctx context.Context, b bus.Bus, pluginID, manifestRoot string) (*PluginResources, error) {
	var lastErr error
	for range pluginResourceConnectAttempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Wait for the target plugin to be loaded and get its SRPC client.
		var pluginClient srpc.Client
		var pluginRef directive.Reference
		var err error
		if manifestRoot == "" {
			pluginClient, pluginRef, err = bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
		} else {
			pluginClient, pluginRef, err = bldr_plugin.ExPluginLoadAtManifestWaitClient(ctx, b, pluginID, manifestRoot)
		}
		if err != nil {
			return nil, errors.Wrap(err, "load plugin")
		}

		// Create a ResourceService client from the plugin's SRPC client.
		resourceSvc := resource.NewSRPCResourceServiceClient(pluginClient)

		// Create the resource client (opens the persistent ResourceClient stream).
		resClient, err := resource_client.NewClient(ctx, resourceSvc)
		if err == nil {
			return &PluginResources{
				Client:    resClient,
				pluginRef: pluginRef,
			}, nil
		}

		pluginRef.Release()
		lastErr = err
		if ctx.Err() != nil || !isTransientPluginResourceClientInitError(err) {
			return nil, errors.Wrap(err, "resource client")
		}
	}

	return nil, errors.Wrapf(
		lastErr,
		"resource client after %d attempts",
		pluginResourceConnectAttempts,
	)
}

func isTransientPluginResourceClientInitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "receive resource client init") {
		return false
	}
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "stream reset") ||
		strings.Contains(msg, "EOF")
}
