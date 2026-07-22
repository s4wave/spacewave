package resource_server_test

import (
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

func TestResourceRPCStreamingMethodPreservesLegacyResourceLifetime(t *testing.T) {
	released := make(chan struct{}, 1)
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != s4wave_world.SRPCWatchWorldStateResourceServiceServiceID ||
			methodID != "WatchWorldState" {
			return false, nil
		}
		if err := strm.MsgRecv(&s4wave_world.WatchWorldStateRequest{}); err != nil {
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
		return true, strm.MsgSend(&s4wave_world.WatchWorldStateResponse{
			ResourceId: resourceID,
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
	stream, err := s4wave_world.NewSRPCWatchWorldStateResourceServiceClient(rootClient).
		WatchWorldState(t.Context(), &s4wave_world.WatchWorldStateRequest{})
	if err != nil {
		t.Fatalf("WatchWorldState: %v", err)
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

	ref := client.CreateResourceReference(resp.GetResourceId())
	ref.Release()
	<-released
}
