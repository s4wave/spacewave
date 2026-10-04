package plugin_space_runtime

import (
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	resource_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// TestBridgeFilterForwardsInfrastructureAndAppPlugins checks each app's parent lookups.
func TestBridgeFilterForwardsInfrastructureAndAppPlugins(t *testing.T) {
	for name, plugins := range map[string][]string{
		"spacewave":  {"spacewave-core", "spacewave-web", "spacewave-app", "web"},
		"first-app":  {"first-shell", "first-storage"},
		"second-app": {"second-host", "second-view"},
	} {
		t.Run(name, func(t *testing.T) { testAppBridge(t, plugins) })
	}
}

// testAppBridge checks the parent boundary for one application declaration.
func testAppBridge(t *testing.T, plugins []string) {
	// Record the directives received by the parent bus.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	recorder := newDirectiveRecorder()
	addTestController(t, parent, recorder)

	// Forward generation demands through the production bridge filter.
	child, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	conf := &plugin_space.Config{EngineId: "engine", VolumeId: "volume"}
	addTestController(t, child, bus_bridge.NewBusBridge(parent, bridgeFilter(conf, plugins)))

	// The Space's own World, its volumes, infrastructure, and declared app plugin
	// lookups reach the parent.
	forwarded := []directive.Directive{
		world.NewLookupWorldEngine("engine"),
		world.NewLookupWorldOp("operation", "engine"),
		objecttype.NewLookupObjectTypeForEngine("test/type", "engine"),
		volume.NewLookupVolume("volume", ""),
		volume.NewBuildObjectStoreAPI("store", "volume"),
		volume.NewLookupVolume(bldr_plugin.PluginVolumeID, ""),
		plugin_host_root.NewLookupRoot([]string{"desktop/darwin/arm64"}),
	}
	for _, id := range plugins {
		forwarded = append(forwarded, bldr_plugin.NewLoadPlugin(id), bldr_plugin.NewLoadPluginInstanced(id, "space-a"))
	}
	for _, dir := range forwarded {
		addTestDirective(t, child, dir)
		recorder.waitFor(t, dir)
	}

	// Other Spaces' Worlds, other volumes, Space plugin loads, manifests, and RPC
	// services stay inside the generation.
	kept := []directive.Directive{
		world.NewLookupWorldEngine("other-engine"),
		world.NewLookupWorldEngine(""),
		world.NewLookupWorldOp("operation", "other-engine"),
		world.NewLookupWorldOp("operation", ""),
		volume.NewLookupVolume("other-volume", ""),
		volume.NewLookupVolume("", ""),
		volume.NewBuildObjectStoreAPI("store", "other-volume"),
		plugin_host.NewLookupPluginHost(nil),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-a"),
		bldr_manifest.NewFetchManifest("plugin", nil, nil, 0),
		bifrost_rpc.NewLookupRpcClient(bldr_plugin.SRPCPluginServiceID, "plugin"),
		bifrost_rpc.NewLookupRpcService(bldr_plugin.SRPCPluginHostServiceID, "plugin-host"),
	}
	for _, dir := range kept {
		addTestDirective(t, child, dir)
	}
	recorder.assertNoMore(t)
}

// addTestController adds ctrl to b until the test ends.
func addTestController(t *testing.T, b bus.Bus, ctrl controller.Controller) {
	t.Helper()

	// Retain the controller until test cleanup.
	release, err := b.AddController(t.Context(), ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

// addTestDirective adds dir to b until the test ends.
func addTestDirective(t *testing.T, b bus.Bus, dir directive.Directive) {
	t.Helper()

	// Retain the directive until test cleanup.
	_, ref, err := b.AddDirective(dir, bus.NewCallbackHandler(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
}

// TestParentFilterForwardsInstallationLoads checks that the parent reaches
// this Space's plugin installation and scheduler and nothing else.
func TestParentFilterForwardsInstallationLoads(t *testing.T) {
	// Bridge a parent bus into a recorded child bus.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Record what reaches the child through the parent filter.
	recorder := newDirectiveRecorder()
	addTestController(t, child, recorder)
	addTestController(t, parent, bus_bridge.NewBusBridge(child, parentFilter("space-a", []string{"app"})))

	// The scheduler lookup and this installation's loads reach the child.
	forwarded := []directive.Directive{
		bldr_plugin.NewLookupPluginScheduler(),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-a"),
	}
	for _, dir := range forwarded {
		addTestDirective(t, parent, dir)
		recorder.waitFor(t, dir)
	}

	// Default, other installation, and app plugin loads stay on the parent.
	kept := []directive.Directive{
		bldr_plugin.NewLoadPlugin("plugin"),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-b"),
		bldr_plugin.NewLoadPluginInstanced("app", "space-a"),
		objecttype.NewLookupObjectTypeForEngine("test/type", "space-a"),
	}
	for _, dir := range kept {
		addTestDirective(t, parent, dir)
	}
	recorder.assertNoMore(t)
}

// TestBridgeScopedObjectTypeLifecycle preserves registry scope, updates, and cancellation.
func TestBridgeScopedObjectTypeLifecycle(t *testing.T) {
	// Start the root registry and its real Resource service.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	parent, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	admission := registration.NewRegistry()
	registry := resource_registry.NewObjectTypeRegistryResource(admission)
	root := srpc.NewMux()
	if err := sdk_registry.SRPCRegisterObjectTypeRegistryResourceService(root, registry); err != nil {
		t.Fatal(err)
	}

	// Expose generation admission through a client with independently owned references.
	if err := admission.Register(root); err != nil {
		t.Fatal(err)
	}
	service := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(service); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(service)))
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Release)

	// Attach the root registry resolver and record the original bridged directive.
	rootRef := resources.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	addTestController(t, parent, resource_registry.NewBridgeController(le, parent, registry))
	recorder := newDirectiveRecorder()
	addTestController(t, parent, recorder)

	// Install both production bridge directions around the child generation.
	child, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	addTestController(t, child, bus_bridge.NewBusBridge(parent, bridgeFilter(&plugin_space.Config{EngineId: "engine-a"}, nil)))
	addTestController(t, parent, bus_bridge.NewBusBridge(child, parentFilter("engine-a", nil)))
	registerBridgedObjectType(t, resources, rootClient, "engine-b", "other")

	// Keep a scoped demand alive before its handler is registered.
	added := make(chan objecttype.ObjectType, 8)
	removed := make(chan objecttype.ObjectType, 8)
	lookup := objecttype.NewLookupObjectTypeForEngine("test/bridged-type", "engine-a")
	inst, ref, err := child.AddDirective(lookup, directive.NewTypedCallbackHandler[objecttype.ObjectType](
		func(value directive.TypedAttachedValue[objecttype.ObjectType]) { added <- value.GetValue() },
		func(value directive.TypedAttachedValue[objecttype.ObjectType]) { removed <- value.GetValue() },
		nil, nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	recorder.waitFor(t, lookup)

	// The parent retains the same directive, including its engine scope.
	var parentInst directive.Instance
	for _, candidate := range parent.GetDirectives() {
		if candidate.GetDirective() == lookup {
			parentInst = candidate
			break
		}
	}
	if parentInst == nil {
		t.Fatal("parent did not retain the original scoped directive")
	}
	if parentDirective(lookup, "engine-a", nil) {
		t.Fatal("typed lookup would be forwarded back into the child")
	}

	// Wait for the root registry to settle the demand in the unrelated scope.
	idle := make(chan struct{}, 1)
	releaseIdle := parentInst.AddIdleCallback(func(settled bool, errs []error) {
		if settled {
			select {
			case idle <- struct{}{}:
			default:
			}
		}
	})
	t.Cleanup(releaseIdle)
	select {
	case <-idle:
	case <-time.After(5 * time.Second):
		t.Fatal("parent lookup did not settle before registration")
	}
	select {
	case <-added:
		t.Fatal("another engine's registration satisfied the scoped demand")
	default:
	}

	// Registration after demand publishes the handler through the child bridge.
	_, withdrawFirst := registerBridgedObjectType(t, resources, rootClient, "engine-a", "first")
	first := waitBridgedObjectType(t, added)
	if first.GetObjectTypeID() != "test/bridged-type" {
		t.Fatalf("bridged type ID = %q", first.GetObjectTypeID())
	}

	// Withdrawing the handler retracts its exact child value while demand remains.
	withdrawFirst()
	if got := waitBridgedObjectType(t, removed); got != first {
		t.Fatal("handler withdrawal removed a different value")
	}

	// A replacement registration satisfies the existing demand with a fresh factory.
	withdrawReplacement, _ := registerBridgedObjectType(t, resources, rootClient, "engine-a", "replacement")
	replacement := waitBridgedObjectType(t, added)
	if replacement == first || replacement.GetObjectTypeID() != first.GetObjectTypeID() {
		t.Fatal("replacement did not publish a fresh factory for the same type")
	}

	// Activating a successor atomically replaces the still-visible handler.
	withdrawSuccessor, _ := registerBridgedObjectType(t, resources, rootClient, "engine-a", "successor")
	if got := waitBridgedObjectType(t, removed); got != replacement {
		t.Fatal("activation removed a different value")
	}
	successor := waitBridgedObjectType(t, added)
	if successor == replacement || successor.GetObjectTypeID() != replacement.GetObjectTypeID() {
		t.Fatal("activation did not publish the successor factory")
	}
	withdrawReplacement()

	// Ending the handler generation also retracts the bridged factory.
	withdrawSuccessor()
	if got := waitBridgedObjectType(t, removed); got != successor {
		t.Fatal("generation withdrawal removed a different value")
	}

	// Cancel the unresolved child demand and wait for parent demand to drain.
	ref.Release()
	select {
	case <-inst.GetContext().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("child lookup survived cancellation")
	}
	select {
	case <-parentInst.GetContext().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("parent lookup survived child cancellation")
	}

	// Registration after cancellation cannot publish another child value.
	registerBridgedObjectType(t, resources, rootClient, "engine-a", "after-cancel")
	select {
	case <-added:
		t.Fatal("canceled lookup received a registered handler")
	default:
	}
}

// registerBridgedObjectType retains an admitted type's Resources until cleanup
// and returns callbacks to withdraw its generation or registration early.
func registerBridgedObjectType(
	t *testing.T,
	resources *resource_client.Client,
	root srpc.Client,
	engineID, manifest string,
) (func(), func()) {
	// Prepare a plugin generation in the consuming engine's installation scope.
	t.Helper()
	prepared, err := sdk_registration.NewSRPCRegistrationServiceClient(root).Prepare(t.Context(), &sdk_registration.PrepareRequest{
		PluginId:     "test-plugin",
		InstanceKey:  engineID,
		ManifestRoot: manifest,
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

	// Register the type privately before publishing its complete generation.
	registered, err := sdk_registry.NewSRPCObjectTypeRegistryResourceServiceClient(scopeClient).RegisterObjectType(t.Context(), &sdk_registry.RegisterObjectTypeRequest{
		TypeId:   "test/bridged-type",
		PluginId: "test-plugin",
	})
	if err != nil {
		t.Fatal(err)
	}
	registrationRef := resources.CreateResourceReference(registered.GetResourceId())
	t.Cleanup(registrationRef.Release)
	if _, err := sdk_registration.NewSRPCGenerationServiceClient(scopeClient).Activate(t.Context(), &sdk_registration.ActivateRequest{}); err != nil {
		t.Fatal(err)
	}
	return scope.Release, registrationRef.Release
}

// waitBridgedObjectType waits for one registry value event with a bounded test deadline.
func waitBridgedObjectType(t *testing.T, events <-chan objecttype.ObjectType) objecttype.ObjectType {
	t.Helper()
	select {
	case value := <-events:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("child did not receive the registry value event")
		return nil
	}
}
