//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	registry := resource_objecttype_registry.NewObjectTypeRegistryResource(nil)
	server := resource_server.NewResourceServer(registry.GetMux())
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	release, err := b.AddController(ctx, newNativeCorePlugin(mux), nil)
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
	proxied := rpcstream.NewRpcStreamClient(bldr_plugin.NewSRPCPluginClient(core).PluginRpc, "glados-core", false)
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
