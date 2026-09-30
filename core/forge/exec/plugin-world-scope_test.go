package space_exec

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// TestPluginExecBorrowedWorldRetainsRegistryScope selects an installed handler through the real borrow route.
func TestPluginExecBorrowedWorldRetainsRegistryScope(t *testing.T) {
	// Register a World object whose handler belongs to one installation only.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	const objectKey = "scoped/object"
	const typeID = "scoped/handler"
	err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		// Create the object and its authoritative graph type together.
		obj, err := ws.CreateObject(ctx, objectKey, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, ws, objectKey, typeID)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Serve the owning registry and generation admission over a real Resource connection.
	generations := registration.NewRegistry()
	registry := resource_registry.NewObjectTypeRegistryResource(generations)
	root := srpc.NewMux(registry.GetMux())
	if err := generations.Register(root); err != nil {
		t.Fatal(err)
	}

	// Connect the registry through its real Resource service.
	registryServer := resource_server.NewResourceServer(root)
	registryMux := resource_server.NewResourceMux(registryServer.Register)
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(registryMux)))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Release)

	// Retain the registry root while preparing the installation generation.
	rootRef := resources.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootRPC, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Prepare and activate an engine-scoped attached handler through registry RPC.
	prepared, err := sdk_registration.NewSRPCRegistrationServiceClient(rootRPC).Prepare(ctx, &sdk_registration.PrepareRequest{
		PluginId: "scoped-plugin", ManifestRoot: "scoped-manifest", InstanceKey: tb.EngineID,
	})
	if err != nil {
		t.Fatal(err)
	}
	scopeRef := resources.CreateResourceReference(prepared.GetResourceId())
	t.Cleanup(scopeRef.Release)
	scopeRPC, err := scopeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Attach the scoped handler to this registry client generation.
	released := make(chan struct{}, 3)
	handlerRoot := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return sdk_registry.SRPCRegisterObjectTypeHandlerService(mux, &scopedObjectTypeHandler{engineID: tb.EngineID, released: released})
	})
	handlerServer := resource_server.NewResourceServer(handlerRoot)
	handlerID, err := resources.AttachResource(ctx, "scoped-handler", resource_server.NewResourceMux(handlerServer.Register))
	if err != nil {
		t.Fatal(err)
	}

	// Publish the scoped handler in the prepared installation generation.
	registered, err := sdk_registry.NewSRPCObjectTypeRegistryResourceServiceClient(scopeRPC).RegisterObjectType(ctx, &sdk_registry.RegisterObjectTypeRequest{
		TypeId: typeID, PluginId: "scoped-plugin", AttachedHandlerResourceId: handlerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registrationRef := resources.CreateResourceReference(registered.GetResourceId())
	t.Cleanup(registrationRef.Release)
	if _, err := sdk_registration.NewSRPCGenerationServiceClient(scopeRPC).Activate(ctx, &sdk_registration.ActivateRequest{}); err != nil {
		t.Fatal(err)
	}

	// Keep the registry's standing directive resolver on the World bus.
	releaseBridge, err := tb.Bus.AddController(ctx, resource_registry.NewBridgeController(tb.Logger, tb.Bus, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseBridge)

	// Mount identity controls selection even when caller context carries another scope.
	for _, engineID := range []string{"", "other-engine", tb.EngineID} {
		// Probe a trusted mount through Resource RPC rather than reading registry state.
		mount := resource_world.NewEngineResource(tb.Logger, tb.Bus, tb.Engine, nil, &sdk_world.EngineInfo{EngineId: engineID})
		t.Cleanup(mount.Close)
		root := srpc.NewMux(mount.GetMux())
		callerEngineID := tb.EngineID
		if engineID == tb.EngineID {
			callerEngineID = "other-engine"
		}
		if err := sdk_world.SRPCRegisterTypedObjectResourceService(root, &scopedTypedObjectService{typed: mount, engineID: callerEngineID}); err != nil {
			t.Fatal(err)
		}

		// Connect the probe to the real Resource server and adopt its root.
		server := resource_server.NewResourceServer(root)
		mux := resource_server.NewResourceMux(server.Register)
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
		if err != nil {
			t.Fatal(err)
		}
		ref := client.AccessRootResource()
		rpc, err := ref.GetClient()
		if err != nil {
			ref.Release()
			client.Release()
			t.Fatal(err)
		}

		// Assert selection through the observable typed child invocation.
		access, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(rpc).AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey})
		if engineID == tb.EngineID {
			if err != nil {
				t.Fatal(err)
			}
			child := client.CreateResourceReference(access.GetResourceId())
			childRPC, err := child.GetClient()
			if err != nil {
				child.Release()
				t.Fatal(err)
			}
			resp, err := echo.NewSRPCEchoerClient(childRPC).Echo(ctx, &echo.EchoMsg{Body: "trusted-mount"})
			child.Release()
			if err != nil {
				t.Fatal(err)
			}
			if resp.GetBody() != "trusted-mount" {
				t.Fatal("trusted mount selected the wrong handler")
			}
		}
		ref.Release()
		client.Release()
		if engineID != tb.EngineID && err == nil {
			t.Fatalf("mount scope %q selected an engine-scoped handler", engineID)
		}
	}

	// A global binding must also mask a scoped parent lifecycle context.
	typed := resource_world.NewTypedObjectResourceWithContext(objecttype.WithEngineID(ctx, tb.EngineID), tb.Logger, tb.Bus, tb.WorldState, tb.Engine, resource_world.WithEngineID(""))
	t.Cleanup(typed.Close)
	globalRoot := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return sdk_world.SRPCRegisterTypedObjectResourceService(mux, typed)
	})
	globalServer := resource_server.NewResourceServer(globalRoot)
	globalMux := resource_server.NewResourceMux(globalServer.Register)
	globalClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(globalMux)))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(globalClient.Release)

	// Observe global absence through the Resource route rather than registry internals.
	globalRef := globalClient.AccessRootResource()
	t.Cleanup(globalRef.Release)
	globalRPC, err := globalRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(globalRPC).AccessTypedObject(ctx, &sdk_world.AccessTypedObjectRequest{ObjectKey: objectKey}); err == nil {
		t.Fatal("global mount inherited the parent lifecycle's scoped handler")
	}

	// Execute through the production bridge and its client-owned attached World tree.
	service := &scopedWorldService{engineID: tb.EngineID, objectKey: objectKey}
	pluginRoot := resource_server.NewResourceMux(func(mux srpc.Mux) error { return SRPCRegisterPluginExecService(mux, service) })
	pluginServer := resource_server.NewResourceServer(pluginRoot)
	pluginMux := resource_server.NewResourceMux(pluginServer.Register)
	client := NewSRPCPluginExecServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux))))
	handler := &pluginExecHandler{
		b: tb.Bus, le: tb.Logger, handle: &pluginExecHandleStub{},
		inputs: forge_target.InputMap{"world": forge_target.NewInputValueWorld(tb.EngineID, tb.Engine, tb.WorldState)},
	}
	if err := handler.executeWithWorld(ctx, client, &PluginExecRequest{}); err != nil {
		t.Fatal(err)
	}

	// Resource release must reach both invocable generations while connections remain open.
	for range 3 {
		select {
		case <-released:
		case <-ctx.Done():
			t.Fatal("borrowed typed child was retained")
		}
	}
	if count := handlerServer.WaitTrackedResourceCount(ctx, 0); count != 0 {
		t.Fatalf("handler retained %d Resources", count)
	}
	if count := pluginServer.WaitTrackedResourceCount(ctx, 0); count != 0 {
		t.Fatalf("plugin retained %d Resources", count)
	}
}
