package flowgraph_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	flowgraph_registry "github.com/s4wave/spacewave/sdk/flowgraph/registry"
)

// Register publishes the Stream Echo compiler and editor through the core Resource client.
// The returned release withdraws both registrations and detaches the compiler.
func Register(ctx context.Context, b bus.Bus, client RegistrationResources, root srpc.Client, editorPath string) (func() error, error) {
	// Require the bundled editor before publishing the plugin's node type.
	if editorPath == "" {
		return nil, errors.New("Stream Echo editor bundle is unavailable")
	}
	nodeType := NewStreamEcho()
	metadata := &flowgraph_registry.NodeType{
		TypeId:      StreamEchoNodeTypeID,
		DisplayName: nodeType.GetDisplayName(),
		Ports:       nodeType.GetPorts(),
		ConfigIds:   nodeType.GetConfigIDs(),
	}
	mux := srpc.NewMux()
	if err := flowgraph_registry.SRPCRegisterNodeTypeResourceService(mux, flowgraph_registry.NewNodeTypeResource(b, metadata, nodeType)); err != nil {
		return nil, err
	}

	// Grant the compiler over the caller's existing Resource stream.
	attachment, err := client.AttachResource(ctx, "Stream Echo compiler", mux)
	if err != nil {
		return nil, err
	}
	refs := make([]resource_client.ResourceRef, 0, 2)
	release := func() error {
		for _, ref := range refs {
			ref.Release()
		}
		return client.DetachResource(ctx, attachment)
	}

	// Publish the editor with the existing config type registry.
	editor, err := s4wave_configtype_registry.NewSRPCConfigTypeRegistryResourceServiceClient(root).
		RegisterConfigType(ctx, &s4wave_configtype_registry.RegisterConfigTypeRequest{
			ConfigId:    stream_echo.ConfigID,
			PluginId:    PluginID,
			DisplayName: nodeType.GetDisplayName(),
			Category:    "Network",
			ScriptPath:  editorPath,
		})
	if err != nil {
		if detachErr := release(); detachErr != nil {
			return nil, errors.Wrapf(err, "detach compiler: %v", detachErr)
		}
		return nil, err
	}
	refs = append(refs, client.CreateResourceReference(editor.GetResourceId()))

	// Retain the compiler declaration until the plugin stops.
	node, err := flowgraph_registry.NewSRPCNodeTypeRegistryResourceServiceClient(root).RegisterNodeType(ctx, &flowgraph_registry.RegisterNodeTypeRequest{
		NodeType:          metadata,
		HandlerResourceId: attachment,
	})
	if err != nil {
		if detachErr := release(); detachErr != nil {
			return nil, errors.Wrapf(err, "detach compiler: %v", detachErr)
		}
		return nil, err
	}
	refs = append(refs, client.CreateResourceReference(node.GetResourceId()))
	return release, nil
}
