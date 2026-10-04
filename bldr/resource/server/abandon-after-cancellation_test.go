package resource_server_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// TestResourceRPCAbandonAfterNestedCancellation retains the route after its
// nested call is canceled, so a later abandonment can release published children.
func TestResourceRPCAbandonAfterNestedCancellation(t *testing.T) {
	// Publish a child before holding the handler until nested cancellation.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	published := make(chan error, 1)
	finished := make(chan struct{})
	released := make(chan struct{}, 1)
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Register the child through the real invocation context.
		defer close(finished)
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		id, err := owner.AddResource(srpc.NewMux(), func() { released <- struct{}{} })
		if err != nil {
			return true, err
		}

		// Complete publication before allowing cancellation to end the handler.
		err = strm.MsgSend(&echo.EchoMsg{Body: strconv.FormatUint(uint64(id), 10)})
		published <- err
		if err != nil {
			return true, err
		}
		<-strm.Context().Done()
		return true, strm.Context().Err()
	})

	// Keep the ResourceClient generation open independently of the raw route.
	server := resource_server.NewResourceServer(rootMux)
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Acquire the generation's root and record its initial resource count.
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Release)
	root := client.AccessRootResource()
	t.Cleanup(root.Release)
	baseline := server.CountTrackedResources()

	// Negotiate a real route whose nested cancellation precedes abandonment.
	route, err := service.ResourceRpc(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = route.Close() })
	if err := route.Send(&resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Init{
		Init: &resource.ResourceRpcInit{ResourceId: root.GetResourceID()},
	}}); err != nil {
		t.Fatal(err)
	}
	ack, err := route.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if ack.GetAck() == nil || ack.GetAck().GetFailure() != nil {
		t.Fatalf("route acknowledgement = %v", ack)
	}

	// Send nested SRPC packets through the same public route transport.
	send := func(packet *srpc.Packet) {
		data, err := packet.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := route.Send(&resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Data{Data: data}}); err != nil {
			t.Fatal(err)
		}
	}
	send(srpc.NewCallStartPacket(echo.SRPCEchoerServiceID, "Echo", nil, true))
	if _, err := route.Recv(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-published:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("child publication did not complete")
	}

	// Let nested cancellation finish the handler before sending abandonment.
	send(srpc.NewCallCancelPacket())
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("nested cancellation did not finish the handler")
	}
	if err := route.Send(&resource.ResourceRpcPacket{Body: &resource.ResourceRpcPacket_Abandon{
		Abandon: &resource.ResourceRpcAbandon{},
	}}); err != nil {
		t.Fatalf("late abandonment: %v", err)
	}
	if err := route.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Release the published child while its ResourceClient generation stays live.
	if got := server.WaitTrackedResourceCount(ctx, baseline); got != baseline {
		t.Fatalf("resource count = %d, want baseline %d", got, baseline)
	}
	select {
	case <-released:
	case <-ctx.Done():
		t.Fatal("abandoned child release callback did not run")
	}
	select {
	case <-client.Done():
		t.Fatal("client generation retired during abandonment")
	default:
	}
}
