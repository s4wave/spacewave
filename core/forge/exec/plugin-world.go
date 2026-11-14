package space_exec

import (
	"context"
	"time"

	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	forge_target "github.com/s4wave/spacewave/forge/target"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// defaultPluginResourceOpenTimeout bounds how long a plugin may take to answer
// the Resource client open after it reported ready. An answering plugin replies
// in milliseconds, so the bound only decides how soon a silent one fails.
const defaultPluginResourceOpenTimeout = 30 * time.Second

// openPluginResources opens the plugin's Resource client within the open bound.
// The bound covers only the open: the returned client lives as long as ctx, so
// a long execution is not cut off. A plugin that stays silent fails with a
// cause naming it, instead of leaving the execution RUNNING forever.
func (h *pluginExecHandler) openPluginResources(ctx context.Context, client SRPCPluginExecServiceClient) (*resource_client.Client, error) {
	// Cancel the open with a cause once the bound passes; the client keeps the
	// derived context, so it is released only when the execution returns.
	timeout := h.resourceOpenTimeout
	if timeout <= 0 {
		timeout = defaultPluginResourceOpenTimeout
	}
	openCtx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(timeout, func() {
		cancel(errors.Errorf(
			"plugin %q did not answer its Resource service within %s: it accepted the connection but never replied to the Resource client open",
			h.conf.GetPluginId(), timeout,
		))
	})
	resources, err := resource_client.NewClient(openCtx, resource.NewSRPCResourceServiceClient(client.SRPCClient()))
	fired := !timer.Stop()
	if err != nil || fired {
		// A bound that passed while the open returned leaves the client retired.
		cause := context.Cause(openCtx)
		if err == nil {
			resources.Release()
			<-resources.Done()
		}
		cancel(nil)
		if fired && ctx.Err() == nil && cause != nil {
			return nil, cause
		}
		return nil, errors.Wrap(err, "connect plugin execution resources")
	}

	// Retire the open context with the client, which owns the stream from here.
	go func() {
		<-resources.Done()
		cancel(nil)
	}()
	return resources, nil
}

// executeWithWorld lends the execution's World through a client-owned Resource
// tree, writing as the execution's plugin. Closing the client withdraws the
// capability and joins outstanding RPCs.
func (h *pluginExecHandler) executeWithWorld(ctx context.Context, client SRPCPluginExecServiceClient, req *PluginExecRequest) error {
	// Resolve the World already granted to this execution.
	input, ok := h.inputs["world"].(forge_target.InputValueWorld)
	if !ok || input.GetWorldEngine() == nil {
		return errors.New("plugin execution requires a transactional World input")
	}
	if err := input.Validate(); err != nil {
		return err
	}
	engine := resource_world.NewEngineResource(
		h.le, h.b, input.GetWorldEngine(), nil, &sdk_world.EngineInfo{EngineId: input.GetWorldEngineID()},
		resource_world.WithWriteGuard(space_world_optypes.NewPluginWriteGuard(h.conf.GetPluginId())),
	)
	defer engine.Close()

	// Open the plugin's Resource service through its execution-service route.
	resources, err := h.openPluginResources(ctx, client)
	if err != nil {
		return err
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
