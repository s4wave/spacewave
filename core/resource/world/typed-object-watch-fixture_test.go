//go:build !js

package resource_world_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	objecttypes "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	sdk_types "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// typedWatchFixture composes a real World mount, registry bridge and Resource connection.
type typedWatchFixture struct {
	// tb owns the in-memory engine and controller bus.
	tb *world_testbed.Testbed
	// resources adopts all returned invocation children.
	resources *resource_client.Client
	// server exposes event-based resource count assertions.
	server *resource_server.ResourceServer
	// rpc serves the engine and registry at the root resource.
	rpc srpc.Client
	// mount owns typed demand on the root engine.
	mount *resource_world.EngineResource
	// wrapWatch optionally gates the real transport for backpressure acceptance.
	wrapWatch func(srpc.Stream) srpc.Stream
}

// newTypedWatchFixture mounts the same World and registry services used by clients.
func newTypedWatchFixture(t *testing.T, engineID string, wrapResourceClient ...func(srpc.Stream) srpc.Stream) *typedWatchFixture {
	// Compose an in-memory engine and the registry's real admission boundary.
	t.Helper()
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Attach the registry bridge to the testbed's existing controller bus.
	groups := registration.NewRegistry()
	registry := objecttypes.NewObjectTypeRegistryResource(groups)
	bridge := objecttypes.NewBridgeController(tb.Logger, tb.Bus, registry)
	releaseBridge, err := tb.Bus.AddController(ctx, bridge, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseBridge)

	// Serve World, preparation and ObjectType registration through real Resource RPC.
	mount := resource_world.NewEngineResource(tb.Logger, tb.Bus, tb.Engine, nil, &sdk_world.EngineInfo{EngineId: engineID})
	t.Cleanup(mount.Close)
	f := &typedWatchFixture{tb: tb, mount: mount}
	root := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		if f.wrapWatch != nil && serviceID == sdk_world.SRPCTypedObjectResourceServiceServiceID && methodID == "WatchTypedObject" {
			stream = f.wrapWatch(stream)
		}
		ctx := objecttype.WithEngineID(stream.Context(), "caller")
		return mount.GetMux().InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(stream, ctx))
	}), registry.GetMux())
	if err := groups.Register(root); err != nil {
		t.Fatal(err)
	}

	// Establish the actual Resource client generation over an in-memory pipe.
	server := resource_server.NewResourceServer(root)
	service := srpc.NewMux()
	if err := server.Register(service); err != nil {
		t.Fatal(err)
	}

	// Gate ResourceClient delivery without changing typed acquisition streams.
	var transport srpc.Invoker = service
	if len(wrapResourceClient) != 0 {
		transport = srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
			if serviceID == resource.SRPCResourceServiceServiceID && methodID == "ResourceClient" {
				stream = wrapResourceClient[0](stream)
			}
			return service.InvokeMethod(serviceID, methodID, stream)
		})
	}
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(transport)))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Release)

	// Retain the root reference that owns the test's RPC surface.
	rootRef := resources.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rpc, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	f.resources, f.server, f.rpc = resources, server, rpc
	return f
}

// setType accepts one object lifecycle change through the actual engine transaction.
func (f *typedWatchFixture) setType(t *testing.T, key, typeID string, remove bool) {
	// Apply creation, retyping or deletion in one World transaction.
	t.Helper()
	tx, err := f.tb.Engine.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	mutateTypedWatchObject(t, tx, key, typeID, remove)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// mutateTypedWatchObject changes only the requested object's normal graph identity.
func mutateTypedWatchObject(t *testing.T, ws world.WorldState, key, typeID string, remove bool) {
	// Remove the object through the World lifecycle operation when requested.
	t.Helper()
	ctx := t.Context()
	if remove {
		if _, err := ws.DeleteObject(ctx, key); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Create the object if necessary and release the independently acquired handle.
	found, err := ws.HasObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Remove or replace the object's type using the normal graph APIs.
	if typeID == "" {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, world_types.TypePred.String(), "", ""), 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range quads {
			if err := ws.DeleteGraphQuad(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
		t.Fatal(err)
	}
}

// register attaches a real handler and transfers its prepared generation to the caller.
// The caller releases the returned reference; the fixture retains the registration child.
func (f *typedWatchFixture) register(t *testing.T, typeID, label, scope string, handler *typedWatchHandler) resource_client.ResourceRef {
	// Attach the handler's real ResourceService to the registry caller.
	t.Helper()
	ctx := t.Context()
	root := srpc.NewMux()
	if err := sdk_types.SRPCRegisterObjectTypeHandlerService(root, handler); err != nil {
		t.Fatal(err)
	}

	// Publish the handler through its own real Resource server.
	handler.server = resource_server.NewResourceServer(root)
	service := srpc.NewMux()
	if err := handler.server.Register(service); err != nil {
		t.Fatal(err)
	}
	attachedID, err := f.resources.AttachResource(ctx, label, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.resources.DetachResource(ctx, attachedID); err != nil && ctx.Err() == nil {
			t.Logf("detach handler after client teardown: %v", err)
		}
	})

	// Prepare a private generation so activation can replace a retained predecessor.
	prepared, err := sdk_registration.NewSRPCRegistrationServiceClient(f.rpc).Prepare(ctx, &sdk_registration.PrepareRequest{
		PluginId: typeID, ManifestRoot: label, InstanceKey: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := f.resources.CreateResourceReference(prepared.GetResourceId())
	rpc, err := ref.GetClient()
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}

	// Register the handler capability beneath this private generation.
	registered, err := sdk_types.NewSRPCObjectTypeRegistryResourceServiceClient(rpc).RegisterObjectType(ctx, &sdk_types.RegisterObjectTypeRequest{
		TypeId: typeID, PluginId: typeID, AttachedHandlerResourceId: attachedID,
	})
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	registration := f.resources.CreateResourceReference(registered.GetResourceId())
	t.Cleanup(registration.Release)

	// Admit the handler only after preparation has registered its complete capability.
	if _, err := sdk_registration.NewSRPCGenerationServiceClient(rpc).Activate(ctx, &sdk_registration.ActivateRequest{}); err != nil {
		ref.Release()
		t.Fatal(err)
	}
	return ref
}

// watch starts a real standing RPC and gives the caller its cancellation function.
func (f *typedWatchFixture) watch(t *testing.T, rpc srpc.Client, key string) (sdk_world.SRPCTypedObjectResourceService_WatchTypedObjectClient, context.CancelFunc) {
	// Open the stream with an independently cancelable request context.
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream, err := sdk_world.NewSRPCTypedObjectResourceServiceClient(rpc).WatchTypedObject(ctx, &sdk_world.WatchTypedObjectRequest{ObjectKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return stream, cancel
}

// receiveTypedWatchSnapshot consumes stream snapshots until the requested availability.
func receiveTypedWatchSnapshot(t *testing.T, stream sdk_world.SRPCTypedObjectResourceService_WatchTypedObjectClient, typeID string, available bool) *sdk_world.WatchTypedObjectResponse {
	// Wait on actual stream events, allowing a replacement's intermediate absence.
	t.Helper()
	for {
		response, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if response.GetTypeId() == typeID && (response.GetResourceId() != 0) == available {
			return response
		}
		if response.GetResourceId() != 0 {
			t.Fatalf("unexpected available generation: %v", response)
		}
	}
}

// child adopts and invokes the real typed resource, transferring its reference to the caller.
// The caller retains it through revocation checks and releases it afterward.
func (f *typedWatchFixture) child(t *testing.T, response *sdk_world.WatchTypedObjectResponse, label string) resource_client.ResourceRef {
	// Adopt the returned child and distinguish its actual handler generation.
	t.Helper()
	ref := f.resources.CreateResourceReference(response.GetResourceId())
	rpc, err := ref.GetClient()
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}

	// Invoke the typed method explicitly and inspect the handler's identity.
	got, err := echo.NewSRPCEchoerClient(rpc).Echo(t.Context(), &echo.EchoMsg{})
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	if got.GetBody() != label {
		ref.Release()
		t.Fatalf("handler = %q, want %q", got.GetBody(), label)
	}
	return ref
}

// rejected proves a revoked adopted reference cannot invoke its old handler.
func (f *typedWatchFixture) rejected(t *testing.T, ref resource_client.ResourceRef) {
	// Invoke through the old resource ID even if its local release notification races.
	t.Helper()
	rpc, err := ref.GetClient()
	if err == nil {
		_, err = echo.NewSRPCEchoerClient(rpc).Echo(t.Context(), &echo.EchoMsg{})
	}
	if err == nil {
		t.Fatal("revoked typed child remained invocable")
	}
}

// count waits on the ResourceServer's owning broadcast while the client stays open.
func (f *typedWatchFixture) count(t *testing.T, want int) {
	// Use a resource revision barrier instead of count polling.
	t.Helper()
	if got := f.server.WaitTrackedResourceCount(t.Context(), want); got != want {
		t.Fatalf("tracked resources = %d, want %d", got, want)
	}
}

// demand observes disposal of the watch's scoped registry directive.
func (f *typedWatchFixture) demand(t *testing.T, typeID, scope string) <-chan struct{} {
	// Register a disposal callback on the already-admitted standing demand.
	t.Helper()
	for _, instance := range f.tb.Bus.GetDirectives() {
		if lookup, ok := instance.GetDirective().(objecttype.LookupObjectType); ok && lookup.LookupObjectTypeID() == typeID && lookup.LookupObjectTypeEngineID() == scope {
			disposed := make(chan struct{})
			release := instance.AddDisposeCallback(func() { close(disposed) })
			t.Cleanup(release)
			return disposed
		}
	}
	t.Fatal("standing ObjectType demand was not retained")
	return nil
}

// waitTypedWatchEvent awaits a lifecycle callback without timers or polling.
func waitTypedWatchEvent(t *testing.T, event <-chan struct{}) {
	// Wait for the producing component or the test's cancellation.
	t.Helper()
	select {
	case <-event:
	case <-t.Context().Done():
		t.Fatal("lifecycle event did not arrive")
	}
}
