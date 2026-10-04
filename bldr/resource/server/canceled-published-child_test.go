package resource_server_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// TestResourceRPCCanceledPublishedChildReturnsCountToBaseline cancels a call
// after a successful server send and before the client adopts the reply.
func TestResourceRPCCanceledPublishedChildReturnsCountToBaseline(t *testing.T) {
	// Register a child and observe the successful send through the real RPC stack.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	sent := make(chan error, 1)
	received := make(chan struct{}, 1)
	released := make(chan struct{}, 1)
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Acquire the handler's resource context after reading its request.
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}

		// Publish a child ID and retain the exact transport send result.
		id, err := owner.AddResource(srpc.NewMux(), func() { released <- struct{}{} })
		if err != nil {
			return true, err
		}
		err = strm.MsgSend(&echo.EchoMsg{Body: strconv.FormatUint(uint64(id), 10)})
		sent <- err
		return true, err
	})

	// Keep the generation open while gating only ResourceRpc response delivery.
	server := resource_server.NewResourceServer(rootMux)
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Retain the generation and root independently of the call being canceled.
	client, err := resource_client.NewClient(ctx, &gatedResponseResourceService{
		SRPCResourceServiceClient: service,
		received:                  received,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Release)
	root := client.AccessRootResource()
	t.Cleanup(root.Release)
	rpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	baseline := server.CountTrackedResources()

	// Start a unary call whose response cannot reach the resource decoder.
	callCtx, cancelCall := context.WithCancel(ctx)
	t.Cleanup(cancelCall)
	done := make(chan error, 1)
	go func() {
		_, err := echo.NewSRPCEchoerClient(rpc).Echo(callCtx, &echo.EchoMsg{})
		done <- err
	}()

	// Establish both successful server send and client transport receipt.
	select {
	case err := <-sent:
		if err != nil {
			t.Fatalf("response send failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("server did not send the response")
	}
	select {
	case <-received:
	case <-ctx.Done():
		t.Fatal("response did not reach the client transport")
	}
	if got := server.CountTrackedResources(); got != baseline+1 {
		t.Fatalf("published count = %d, want %d", got, baseline+1)
	}

	// Cancel before response decoding and require release without disconnecting.
	cancelCall()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("call error = %v, want context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatal("canceled call did not finish")
	}

	// Require child cleanup before the generation's own deadline expires.
	releaseCtx, cancelRelease := context.WithTimeout(ctx, time.Second)
	t.Cleanup(cancelRelease)
	got := server.WaitTrackedResourceCount(releaseCtx, baseline)
	select {
	case <-client.Done():
		t.Fatal("client generation retired during child cleanup")
	default:
	}
	if got != baseline {
		t.Fatalf("resource count = %d, want baseline %d", got, baseline)
	}
	select {
	case <-released:
	case <-ctx.Done():
		t.Fatal("unadopted child release callback did not run")
	}
}
