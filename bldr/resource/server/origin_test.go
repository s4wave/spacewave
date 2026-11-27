package resource_server_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// TestResourceCallOrigin checks a resource call records the server's origin
// controller. The client generation the call runs in records that
// controller too.
func TestResourceCallOrigin(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Record the origins the handler sees on its call and generation contexts.
	type seen struct{ call, client directive.Origin }
	seenCh := make(chan seen, 1)
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Read the request.
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}

		// Report both origins and reply.
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		seenCh <- seen{
			call:   directive.OriginFromContext(strm.Context()),
			client: directive.OriginFromContext(owner.Context()),
		}
		return true, strm.MsgSend(&echo.EchoMsg{})
	})

	// Serve the root with an origin controller.
	const originID = "test/resource-root"
	server := resource_server.NewResourceServer(rootMux)
	server.SetOriginController(originID)
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}

	// Connect a client.
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()

	// Call the root once.
	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rpc, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := echo.NewSRPCEchoerClient(rpc).Echo(ctx, &echo.EchoMsg{}); err != nil {
		t.Fatal(err)
	}

	// Check both contexts record the origin controller.
	got := <-seenCh
	if got.call.ControllerID != originID {
		t.Fatalf("call origin = %q, want %q", got.call.ControllerID, originID)
	}
	if got.client.ControllerID != originID {
		t.Fatalf("generation origin = %q, want %q", got.client.ControllerID, originID)
	}
}
