package bldr_plugin

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

func TestPluginServerResourceServiceFallsThroughToBus(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	le := logrus.NewEntry(log)
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	targetMux := srpc.NewMux()
	if err := resource.SRPCRegisterResourceService(targetMux, sentinelResourceService{}); err != nil {
		t.Fatal(err)
	}
	ctrl := bifrost_rpc.NewInvokerController(
		le,
		b,
		controller.NewInfo("bldr/plugin/test-resource-service", controller.MustParseVersion("0.0.1"), ""),
		targetMux,
		[]string{resource.SRPCResourceServiceServiceID},
	)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	pluginMux := srpc.NewMux()
	if err := SRPCRegisterPlugin(pluginMux, NewPluginServer(b)); err != nil {
		t.Fatal(err)
	}
	pluginClient := NewSRPCPluginClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux))))
	openStream := rpcstream.NewRpcStreamClient(pluginClient.PluginRpc, "spacewave-app", true)
	resClient := resource.NewSRPCResourceServiceClient(openStream)

	strm, err := resClient.ResourceClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	if err := strm.Send(&resource.ResourceClientRequest{
		Body: &resource.ResourceClientRequest_Init{
			Init: &resource.ResourceClientInitRequest{},
		},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := strm.Recv()
	if err != nil {
		t.Fatal(err)
	}
	init := resp.GetInit()
	if init == nil {
		t.Fatalf("expected init response, got %T", resp.GetBody())
	}
	if init.GetClientHandleId() != 88 || init.GetRootResourceId() != 99 {
		t.Fatalf("expected bus resource service init, got client=%d root=%d", init.GetClientHandleId(), init.GetRootResourceId())
	}
}

type sentinelResourceService struct{}

func (sentinelResourceService) ResourceClient(
	strm resource.SRPCResourceService_ResourceClientStream,
) error {
	req, err := strm.Recv()
	if err != nil {
		return err
	}
	if req.GetInit() == nil {
		return errors.New("expected resource client init")
	}
	if err := strm.Send(&resource.ResourceClientResponse{
		Body: &resource.ResourceClientResponse_Init{
			Init: &resource.ResourceClientInit{
				ClientHandleId: 88,
				RootResourceId: 99,
			},
		},
	}); err != nil {
		return err
	}
	<-strm.Context().Done()
	return context.Canceled
}

func (sentinelResourceService) ResourceRpc(resource.SRPCResourceService_ResourceRpcStream) error {
	return errors.New("unexpected resource rpc")
}

func (sentinelResourceService) ResourceAttach(resource.SRPCResourceService_ResourceAttachStream) error {
	return errors.New("unexpected resource attach")
}
