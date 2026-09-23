//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
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

// A plugin host can reach the native registry without loading a core plugin.
func TestNativeCoreResourceRouteUsesLocalRegistry(t *testing.T) {
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
	release, err := registerNativeCoreResource(ctx, b, mux)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b,
		bldr_plugin.PluginServiceID("spacewave-core", resource.SRPCResourceServiceServiceID), "", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(invokers) != 1 {
		t.Fatalf("core Resource routes = %d, want 1", len(invokers))
	}
	client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0])))))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()
	root := client.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk_registry.NewSRPCObjectTypeRegistryResourceServiceClient(rootClient).RegisterObjectType(ctx,
		&sdk_registry.RegisterObjectTypeRequest{TypeId: "test/native-type", PluginId: "test-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	registration := client.CreateResourceReference(result.GetResourceId())
	defer registration.Release()
	if result.GetResourceId() == 0 {
		t.Fatal("native registry did not retain the plugin's type registration")
	}
}
