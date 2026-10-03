//go:build !js

package plugin_space_runtime

import (
	"context"
	"testing"
	"time"

	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// TestBridgeAttachedWorldWatch invokes a parent handler registered after child demand.
func TestBridgeAttachedWorldWatch(t *testing.T) {
	// Bound the real Resource calls and retain an in-memory parent World.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Commit a typed object before any handler is registered.
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	obj, err := tx.CreateObject(ctx, "test/bridged-object", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Assign the object's type in the same committed World transaction.
	if err := world_types.SetObjectType(ctx, tx, "test/bridged-object", "test/bridged-type"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Keep root registrations on the parent and mount the World on a child bus.
	admission := registration.NewRegistry()
	registry := resource_registry.NewObjectTypeRegistryResource(admission)
	addTestController(t, tb.Bus, resource_registry.NewBridgeController(tb.Logger, tb.Bus, registry))
	child, _, err := controllerbus_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}
	addTestController(t, child, bus_bridge.NewBusBridge(tb.Bus, bridgeFilter(nil)))
	mount := resource_world.NewEngineResource(tb.Logger, child, tb.Engine, nil, &sdk_world.EngineInfo{EngineId: tb.EngineID})
	t.Cleanup(mount.Close)

	// Serve a consumer that watches the World attached by the execution caller.
	absent := make(chan struct{}, 1)
	var attachedEngineID uint32
	root := srpc.NewMux(registry.GetMux(), srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		// Match the consumer call before observing its attached World.
		if serviceID != echo.SRPCEchoerServiceID || methodID != "Echo" {
			return false, nil
		}
		if err := stream.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}

		// Invoke the child only after its standing watch publishes a handler.
		response, err := invokeBridgedWorldWatch(stream.Context(), attachedEngineID, absent)
		if err != nil {
			return true, err
		}
		return true, stream.MsgSend(response)
	}))
	if err := admission.Register(root); err != nil {
		t.Fatal(err)
	}

	// Connect the real Resource client to the parent's registration service.
	service := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(service); err != nil {
		t.Fatal(err)
	}
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(service)))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Release)

	// Retain the root registration and consumer RPC surface until cleanup.
	rootRef := resources.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Lend the exact child-bus World through the production attachment API.
	attachedEngineID, err = resources.AttachResourceTree(ctx, "execution-world", mount.GetMux())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		response, err := echo.NewSRPCEchoerClient(rootClient).Echo(ctx, &echo.EchoMsg{})
		if err == nil && response.GetBody() != "parent-handler" {
			err = errors.Errorf("child handler response = %q", response.GetBody())
		}
		done <- err
	}()
	select {
	case <-absent:
	case err := <-done:
		t.Fatalf("consumer ended before handler registration: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Attach the parent's real handler and prepare its engine-scoped generation.
	handlerRoot := srpc.NewMux()
	if err := sdk_registry.SRPCRegisterObjectTypeHandlerService(handlerRoot, &bridgedWorldHandler{engineID: tb.EngineID}); err != nil {
		t.Fatal(err)
	}
	handlerService := srpc.NewMux()
	if err := resource_server.NewResourceServer(handlerRoot).Register(handlerService); err != nil {
		t.Fatal(err)
	}
	handlerID, err := resources.AttachResource(ctx, "parent-handler", handlerService)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the parent's handler in the exact child engine scope.
	prepared, err := sdk_registration.NewSRPCRegistrationServiceClient(rootClient).Prepare(ctx, &sdk_registration.PrepareRequest{
		PluginId: "test-plugin", InstanceKey: tb.EngineID, ManifestRoot: "watch-handler",
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := resources.CreateResourceReference(prepared.GetResourceId())
	t.Cleanup(scope.Release)
	scopeClient, err := scope.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Publish the handler after the child has reported standing typed absence.
	registered, err := sdk_registry.NewSRPCObjectTypeRegistryResourceServiceClient(scopeClient).RegisterObjectType(ctx, &sdk_registry.RegisterObjectTypeRequest{
		TypeId: "test/bridged-type", PluginId: "test-plugin", AttachedHandlerResourceId: handlerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	registrationRef := resources.CreateResourceReference(registered.GetResourceId())
	t.Cleanup(registrationRef.Release)
	if _, err := sdk_registration.NewSRPCGenerationServiceClient(scopeClient).Activate(ctx, &sdk_registration.ActivateRequest{}); err != nil {
		t.Fatal(err)
	}

	// Await the watch event and explicit invocation through the attached World.
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("child World did not receive and invoke the parent handler: ", ctx.Err())
	}
}

// invokeBridgedWorldWatch retains a standing attached watch until one child is invoked.
func invokeBridgedWorldWatch(ctx context.Context, engineID uint32, absent chan<- struct{}) (*echo.EchoMsg, error) {
	// Open typed demand on the exact World supplied by the caller.
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineRPC, err := owner.GetAttachedResource(engineID)
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watch, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(engineRPC).WatchTypedObject(watchCtx, &sdk_world.WatchTypedObjectRequest{ObjectKey: "test/bridged-object"})
	if err != nil {
		return nil, err
	}

	// Require typed absence before the parent registers its handler.
	response, err := watch.Recv()
	if err != nil {
		return nil, err
	}
	if response.GetTypeId() != "test/bridged-type" || response.GetResourceId() != 0 {
		return nil, errors.Errorf("initial typed watch snapshot = %v", response)
	}
	absent <- struct{}{}

	// Receive the registration event and adopt its independently owned child.
	response, err = watch.Recv()
	if err != nil {
		return nil, err
	}
	if response.GetTypeId() != "test/bridged-type" || response.GetResourceId() == 0 {
		return nil, errors.Errorf("registered typed watch snapshot = %v", response)
	}
	ref := sdk_world_engine.NewAttachedResourceClient(owner).CreateResourceReference(response.GetResourceId())
	defer ref.Release()
	client, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return echo.NewSRPCEchoerClient(client).Echo(ctx, &echo.EchoMsg{Body: "parent-handler"})
}
