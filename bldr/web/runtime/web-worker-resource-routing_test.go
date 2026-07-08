//go:build !js

package web_runtime_test

import (
	"io"
	"regexp"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	"github.com/sirupsen/logrus"
)

func TestGetWebWorkerHostRoutesResourceClientToWorkerScopedTestbedRoot(t *testing.T) {
	ctx := t.Context()
	workerID := "plugin/alternate-resource-proof"
	workerServerID := "web-worker/" + workerID

	log := logrus.New()
	log.SetOutput(io.Discard)
	le := logrus.NewEntry(log)
	bus, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	testbedRoot := resource_testbed.NewTestbedResourceServer(
		ctx,
		le,
		bus,
		"alternate-worker-proof-volume",
		"alternate-worker-proof-bucket",
	)
	rootResourceMux := srpc.NewMux()
	if err := testbedRoot.Register(rootResourceMux); err != nil {
		t.Fatalf("register testbed root resource: %v", err)
	}

	workerHostMux := srpc.NewMux()
	workerResourceServer := resource_server.NewResourceServer(rootResourceMux)
	if err := workerResourceServer.Register(workerHostMux); err != nil {
		t.Fatalf("register worker resource server: %v", err)
	}
	workerCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			"bldr/web/runtime/test-worker-resource-host",
			controller.MustParseVersion("0.0.1"),
			"test web worker resource host",
		),
		bifrost_rpc.NewRpcServiceBuilder(workerHostMux),
		nil,
		false,
		nil,
		[]string{bldr_resource.SRPCResourceServiceServiceID},
		regexp.MustCompile("^"+regexp.QuoteMeta(workerServerID)+"$"),
	)
	workerRelease, err := bus.AddController(ctx, workerCtrl, nil)
	if err != nil {
		t.Fatalf("add worker resource controller: %v", err)
	}
	defer workerRelease()

	poisonHostMux := srpc.NewMux()
	poisonResourceServer := resource_server.NewResourceServer(srpc.NewMux())
	if err := poisonResourceServer.Register(poisonHostMux); err != nil {
		t.Fatalf("register poison resource server: %v", err)
	}
	poisonCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			"bldr/web/runtime/test-unprefixed-worker-resource-host",
			controller.MustParseVersion("0.0.1"),
			"test unprefixed web worker resource host",
		),
		bifrost_rpc.NewRpcServiceBuilder(poisonHostMux),
		nil,
		false,
		nil,
		[]string{bldr_resource.SRPCResourceServiceServiceID},
		regexp.MustCompile("^"+regexp.QuoteMeta(workerID)+"$"),
	)
	poisonRelease, err := bus.AddController(ctx, poisonCtrl, nil)
	if err != nil {
		t.Fatalf("add unprefixed worker resource controller: %v", err)
	}
	defer poisonRelease()

	remote, err := web_runtime.NewRemote(le, bus, nil, "alt-worker-proof", nil, nil)
	if err != nil {
		t.Fatalf("new web runtime remote: %v", err)
	}
	workerHost, releaseWorkerHost, err := remote.GetWebWorkerHost(ctx, workerID, nil)
	if err != nil {
		t.Fatalf("get web worker host: %v", err)
	}
	if releaseWorkerHost != nil {
		defer releaseWorkerHost()
	}

	workerServiceClient := bldr_resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(workerHost))),
	)
	resources, err := resource_client.NewClient(ctx, workerServiceClient)
	if err != nil {
		t.Fatalf("new worker resource client: %v", err)
	}
	defer resources.Release()

	rootRef := resources.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatalf("root resource client: %v", err)
	}
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient)
	const resultMessage = "alternate worker resource proof reached testbed root"
	_, err = testbedClient.MarkTestResult(ctx, &s4wave_testbed.MarkTestResultRequest{
		Success:      true,
		ErrorMessage: resultMessage,
	})
	if err != nil {
		t.Fatalf("mark test result through worker root: %v", err)
	}

	success, errorMessage, err := testbedRoot.WaitForTestResult(ctx)
	if err != nil {
		t.Fatalf("wait for testbed result: %v", err)
	}
	if !success || errorMessage != resultMessage {
		t.Fatalf("testbed root result = (%t, %q), want (true, %q)", success, errorMessage, resultMessage)
	}
}
