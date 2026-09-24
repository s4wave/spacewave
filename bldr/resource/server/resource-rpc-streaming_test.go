package resource_server_test

import (
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

func TestResourceRPCStreamingMethodPreservesLegacyResourceLifetime(t *testing.T) {
	released := make(chan struct{}, 1)
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != echo.SRPCEchoerServiceID || methodID != "EchoServerStream" {
			return false, nil
		}
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		resourceID, err := owner.AddResource(srpc.NewMux(), func() {
			released <- struct{}{}
		})
		if err != nil {
			return true, err
		}
		return true, strm.MsgSend(&echo.EchoMsg{
			Body: strconv.FormatUint(uint64(resourceID), 10),
		})
	}))
	server := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := server.Register(serverMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	service := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))),
	)
	client, err := resource_client.NewClient(t.Context(), service)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Release()

	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatalf("root client: %v", err)
	}
	stream, err := echo.NewSRPCEchoerClient(rootClient).
		EchoServerStream(t.Context(), &echo.EchoMsg{})
	if err != nil {
		t.Fatalf("EchoServerStream: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("receive streamed resource: %v", err)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream completion: got %v, want EOF", err)
	}
	select {
	case <-released:
		t.Fatal("streamed resource was released at stream completion")
	default:
	}

	resourceID, err := strconv.ParseUint(resp.GetBody(), 10, 32)
	if err != nil {
		t.Fatalf("parse streamed resource id: %v", err)
	}
	ref := client.CreateResourceReference(uint32(resourceID))
	ref.Release()
	<-released
}
