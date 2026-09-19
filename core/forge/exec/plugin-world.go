package space_exec

import (
	"context"

	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// executeWithWorld lends the execution's World through a client-owned Resource
// tree. Closing the client withdraws the capability and joins outstanding RPCs.
func (h *pluginExecHandler) executeWithWorld(ctx context.Context, client SRPCPluginExecServiceClient, req *PluginExecRequest) error {
	// Resolve the World already granted to this execution.
	input, ok := h.inputs["world"].(forge_target.InputValueWorld)
	if !ok || input.GetWorldEngine() == nil {
		return errors.New("plugin execution requires a transactional World input")
	}
	engine := resource_world.NewEngineResource(h.le, h.b, input.GetWorldEngine(), nil, nil)
	defer engine.Close()

	// Open the plugin's Resource service through its execution-service route.
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client.SRPCClient()))
	if err != nil {
		return errors.Wrap(err, "connect plugin execution resources")
	}
	defer func() {
		resources.Release()
		<-resources.Done()
	}()
	root := resources.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		return err
	}

	// Attach the engine for this request only; persisted targets contain no IDs.
	engineID, err := resources.AttachResourceTree(ctx, "forge-world", engine.GetMux())
	if err != nil {
		return errors.Wrap(err, "attach plugin execution World")
	}
	req.AttachedEngineResourceId = engineID
	return h.executeClient(ctx, NewSRPCPluginExecServiceClient(rootClient), req)
}
