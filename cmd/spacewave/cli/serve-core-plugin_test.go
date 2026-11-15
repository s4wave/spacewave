//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/sirupsen/logrus"
)

// Plugins reach the native registry through a core plugin load or the
// qualified core service route, without a core plugin process.
func TestNativeCorePluginUsesLocalRegistry(t *testing.T) {
	// Open a core bus with a timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}

	// Register the object-type registry and add the native core plugin.
	registry := resource_objecttype_registry.NewObjectTypeRegistryResource(nil)
	server := resource_server.NewResourceServer(registry.GetMux())
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	release, err := b.AddController(ctx, newNativeCorePlugin(b, mux), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// A Space plugin loads core under its own instance key, then calls it
	// through the Plugin service as the host's PluginRpc proxy does.
	core, coreRef, err := bldr_plugin.ExPluginLoadInstancedWaitClient(ctx, b, nativeCorePluginID, "space-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer coreRef.Release()
	proxied := rpcstream.NewRpcStreamClient(bldr_plugin.NewSRPCPluginClient(core).PluginRpc, (&bldr_plugin.PluginCaller{PluginId: "gizmo-core", BindingKey: "space-a"}).ComponentID(), false)
	registerNativeTestType(ctx, t, proxied, "test/plugin-type")

	// A host resolves the qualified core Resource route.
	invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b,
		bldr_plugin.PluginServiceID(nativeCorePluginID, resource.SRPCResourceServiceServiceID), "", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(invokers) != 1 {
		t.Fatalf("core Resource routes = %d, want 1", len(invokers))
	}
	routed := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0])))
	registerNativeTestType(ctx, t, routed, "test/route-type")
}

// registerNativeTestType registers typeID in the core registry over client and
// checks that the registry retains it.
func registerNativeTestType(ctx context.Context, t *testing.T, client srpc.Client, typeID string) {
	// Open the resource client and its root.
	t.Helper()
	resClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resClient.Release)
	root := resClient.AccessRootResource()
	t.Cleanup(root.Release)
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Register the test type and retain its resource.
	result, err := sdk_registry.NewSRPCObjectTypeRegistryResourceServiceClient(rootClient).RegisterObjectType(ctx,
		&sdk_registry.RegisterObjectTypeRequest{TypeId: typeID, PluginId: "test-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetResourceId() == 0 {
		t.Fatalf("native registry did not retain %s", typeID)
	}
	registration := resClient.CreateResourceReference(result.GetResourceId())
	t.Cleanup(registration.Release)
}

// TestNativeCorePluginRefusesPluginServers checks native core hands its root
// client to no lookup a plugin made. The daemon's root scheduler runs no host
// plugin, so every "plugin/" server ID is refused at once, while the host's own
// lookup still reaches core.
func TestNativeCorePluginRefusesPluginServers(t *testing.T) {
	// Open a core bus with a timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}

	// Add native core over the registry and the daemon's root scheduler.
	registry := resource_objecttype_registry.NewObjectTypeRegistryResource(nil)
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(registry.GetMux()).Register(mux); err != nil {
		t.Fatal(err)
	}
	for _, ctrl := range []controller.Controller{newNativeCorePlugin(b, mux), &daemonRootScheduler{}} {
		release, err := b.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}

	// A lookup under a plugin's server ID is refused at once.
	serviceID := bldr_plugin.PluginServiceID(nativeCorePluginID, resource.SRPCResourceServiceServiceID)
	for _, serverID := range []string{"plugin/attacker", "plugin/" + nativeCorePluginID + "/sub", "web-worker/plugin/attacker"} {
		start := time.Now()
		_, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b, serviceID, serverID, true, nil)
		if ref != nil {
			ref.Release()
		}
		if !errors.Is(err, bldr_plugin.ErrRelayRefused) || time.Since(start) > 500*time.Millisecond {
			t.Errorf("lookup as %q: want a refusal at once, got %v after %s", serverID, err, time.Since(start))
		}
	}

	// The host's own lookup still reaches core.
	invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b, serviceID, "", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(invokers) != 1 {
		t.Fatalf("core Resource routes = %d, want 1", len(invokers))
	}
	registerNativeTestType(ctx, t, srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))), "test/host-type")
}

// daemonRootScheduler stands for the daemon's root plugin scheduler, which
// runs no host plugin.
type daemonRootScheduler struct{}

func (s *daemonRootScheduler) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/daemon-root-scheduler", controller.MustParseVersion("0.0.1"), "")
}

func (s *daemonRootScheduler) Execute(context.Context) error {
	return nil
}

func (s *daemonRootScheduler) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

func (s *daemonRootScheduler) Close() error {
	return nil
}

func (s *daemonRootScheduler) GetInstanceKey() string {
	return ""
}

func (s *daemonRootScheduler) GetPluginStatusCtr() ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] {
	return nil
}

func (s *daemonRootScheduler) GetHostPluginIds() []string {
	return nil
}
