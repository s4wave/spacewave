package web_pkg_rpc_e2e

import (
	"context"
	"testing"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bifrost_rpc_access "github.com/aperturerobotics/bifrost/rpc/access"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_controller "github.com/aperturerobotics/bldr/web/pkg/controller"
	web_pkg_mock "github.com/aperturerobotics/bldr/web/pkg/mock"
	web_pkg_rpc "github.com/aperturerobotics/bldr/web/pkg/rpc"
	web_pkg_rpc_client "github.com/aperturerobotics/bldr/web/pkg/rpc/client"
	web_pkg_rpc_server "github.com/aperturerobotics/bldr/web/pkg/rpc/server"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// TestWebPkgRpc tests the web pkg rpc server and client.
func TestWebPkgRpc(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the first testbed (client).
	tb1, err := testbed.NewTestbed(ctx, le.WithField("testbed", "tb1"))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb1.StaticResolver.AddFactory(web_pkg_rpc_client.NewFactory(tb1.Bus))

	// Build the second testbed (server).
	tb2, err := testbed.NewTestbed(ctx, le.WithField("testbed", "tb2"))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb2.StaticResolver.AddFactory(web_pkg_rpc_server.NewFactory(tb2.Bus))

	// Construct the resolver for LookupWebPkg on the server.
	testPkgID, testPkgIDPrefix := web_pkg_mock.MockWebPkgID, web_pkg_mock.MockWebPkgIDPrefix
	mockCtrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/rpc/e2e/static-pkg", semver.MustParse("0.0.1"), "static pkg"),
		web_pkg_mock.NewMockWebPkg(),
	)
	relMock, err := tb2.Bus.AddController(ctx, mockCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relMock()

	// Construct the server.
	_, _, srvRef, err := loader.WaitExecControllerRunning(ctx, tb2.Bus, resolver.NewLoadControllerWithConfig(&web_pkg_rpc_server.Config{
		ServiceIdPrefix:  web_pkg_rpc.DefServiceIDPrefix,
		WebPkgIdPrefixes: []string{testPkgIDPrefix},
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer srvRef.Release()

	// construct the rpc mux
	rpcMux := srpc.NewMux(bifrost_rpc.NewInvoker(tb2.Bus, "default", true))

	// handle AccessRpcService requests via bus LookupRpcService.
	accessRpcServiceServer := bifrost_rpc_access.NewAccessRpcServiceServer(
		tb2.Bus,
		true,
		nil,
	)
	_ = bifrost_rpc_access.SRPCRegisterAccessRpcService(rpcMux, accessRpcServiceServer)

	// Construct the srpc server.
	srv := srpc.NewServer(rpcMux)

	// Construct the srpc client.
	client := srpc.NewClient(srpc.NewServerPipe(srv))

	// Execute the rpc client controller
	testServiceIDPrefix := "test-remote-server/"
	clientCtrl := bifrost_rpc.NewClientController(
		le,
		tb1.Bus,
		controller.NewInfo("web/pkg/rpc/e2e/client", semver.MustParse("0.0.1"), "rpc e2e client"),
		client,
		[]string{testServiceIDPrefix},
	)
	relSrpcClient, err := tb1.Bus.AddController(ctx, clientCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relSrpcClient()

	// Execute the web_pkg_rpc client.
	_, _, clientRef, err := loader.WaitExecControllerRunning(ctx, tb1.Bus, resolver.NewLoadControllerWithConfig(&web_pkg_rpc_client.Config{
		ServiceIdPrefix:  testServiceIDPrefix + web_pkg_rpc.DefServiceIDPrefix,
		ClientId:         "test-client",
		WebPkgIdPrefixes: []string{testPkgIDPrefix},
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer clientRef.Release()

	// Execute the LookupWebPkg directive on the client end.
	val, _, valRef, err := web_pkg.ExLookupWebPkg(ctx, tb1.Bus, false, testPkgID)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer valRef.Release()

	if val.GetId() != testPkgID {
		t.Fatalf("value id wrong: %s != %s", val.GetId(), testPkgID)
	}

	info, err := val.GetInfo(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if info.GetId() != testPkgID {
		t.Fatalf("get info returned wrong id: %s != %s", info.GetId(), testPkgID)
	}
}
