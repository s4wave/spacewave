package resource_objecttype_registry

import (
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// TestBridgeResolverDoesNotReplayAcceptedPluginMethod preserves an accepted call's failure.
func TestBridgeResolverDoesNotReplayAcceptedPluginMethod(t *testing.T) {
	// Start the in-memory World and registry bus for the plugin bridge.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Expose distinguishable real plugin Resource services at the acceptance boundary.
	first := &typedMethodPlugin{invokeErr: resource.ErrResourceOrClientReleased}
	firstClient, firstServer := first.client(t)
	next := &typedMethodPlugin{}
	nextClient, nextServer := next.client(t)
	loader := &testPluginLoadController{client: firstClient}
	first.afterAccept = func() { loader.SetClient(nextClient) }

	// Admit the plugin registration through the existing registry bridge fixture.
	release, err := tb.Bus.AddController(ctx, loader, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Admit the registered ObjectType independently of the plugin load resolver.
	registry := NewObjectTypeRegistryResource(nil)
	registry.registrations[1] = &objectTypeRegistration{registration: &sdk_registry.ObjectTypeRegistration{
		TypeId: "test/type", RegistrationId: 1, PluginId: "test-plugin",
	}}
	bridge := NewBridgeController(tb.Logger, tb.Bus, registry)
	release, err = tb.Bus.AddController(ctx, bridge, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Construct the same plugin factory available to a typed World watch.
	objectType, ref, err := objecttype.ExLookupObjectType(ctx, tb.Bus, "test/type")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	invoker, cleanup, err := objectType.GetFactory()(ctx, tb.Logger, tb.Bus, tb.Engine, nil, "test/object")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	// Join the forwarding method through the normal Invoker boundary after its error.
	completed := make(chan error, 1)
	forwarder := srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		found, err := invoker.InvokeMethod(serviceID, methodID, stream)
		completed <- err
		return found, err
	})
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(forwarder)))
	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err == nil {
		t.Fatal("accepted plugin operation lost its released-resource failure")
	}
	operationErr := <-completed

	// The failed explicit call must neither invoke nor open a successor generation.
	if factories, invocations, sessions := first.counts(); factories != 1 || invocations != 1 || sessions != 1 {
		t.Fatalf("first plugin counts = (%d, %d, %d), want (1, 1, 1)", factories, invocations, sessions)
	}
	if factories, invocations, sessions := next.counts(); factories != 0 || invocations != 0 || sessions != 0 {
		t.Fatalf("successor plugin counts after failed call = (%d, %d, %d), want (0, 0, 0)", factories, invocations, sessions)
	}
	if operationErr == nil || operationErr.Error() != resource.ErrResourceOrClientReleased.Error() {
		t.Fatalf("forwarded operation error = %v, want released-resource failure", operationErr)
	}

	// Release the failed child and verify its Resource generation drains completely.
	cleanup()
	if count := firstServer.WaitTrackedResourceCount(ctx, 0); count != 0 {
		t.Fatalf("first plugin retained %d resources after cleanup", count)
	}

	// A fresh explicit factory can resolve and invoke the successor normally.
	replacement, releaseReplacement, err := objectType.GetFactory()(ctx, tb.Logger, tb.Bus, tb.Engine, nil, "test/object")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseReplacement)
	replacementClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(replacement)))
	if err := replacementClient.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("explicit successor operation: %v", err)
	}
	if factories, invocations, sessions := next.counts(); factories != 1 || invocations != 1 || sessions != 1 {
		t.Fatalf("explicit successor counts = (%d, %d, %d), want (1, 1, 1)", factories, invocations, sessions)
	}

	// Release the independently acquired replacement while the test services remain open.
	releaseReplacement()
	if count := nextServer.WaitTrackedResourceCount(ctx, 0); count != 0 {
		t.Fatalf("successor plugin retained %d resources after cleanup", count)
	}
}
