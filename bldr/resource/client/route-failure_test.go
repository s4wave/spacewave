package resource_client

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// lifecyclePhrases are diagnostics that once selected lifecycle decisions.
var lifecyclePhrases = []string{
	"client was released",
	"resource not found",
	"invalid resource id",
	"resource or client was released",
}

// rewordedRouteFailure replaces every route refusal diagnostic in transit.
const rewordedRouteFailure = "the selected handle is gone"

// rewordingResourceServer serves real routes with reworded refusal text.
type rewordingResourceServer struct {
	*resource_server.ResourceServer
}

// ResourceRpc routes through the real server and rewrites its acknowledgement.
func (s *rewordingResourceServer) ResourceRpc(strm resource.SRPCResourceService_ResourceRpcStream) error {
	return s.ResourceServer.ResourceRpc(&rewordingRouteStream{SRPCResourceService_ResourceRpcStream: strm})
}

// rewordingRouteStream rewrites route failure messages and keeps their codes.
type rewordingRouteStream struct {
	resource.SRPCResourceService_ResourceRpcStream
}

// Send replaces the refusal diagnostic before the acknowledgement leaves.
func (s *rewordingRouteStream) Send(packet *resource.ResourceRpcPacket) error {
	if failure := packet.GetAck().GetFailure(); failure != nil {
		failure.Message = rewordedRouteFailure
	}
	return s.SRPCResourceService_ResourceRpcStream.Send(packet)
}

// lifecycleTestInvoker serves Ping and fails Fail with the requested text.
func lifecycleTestInvoker(serviceID string) srpc.Invoker {
	return srpc.InvokerFunc(func(gotService, method string, strm srpc.Stream) (bool, error) {
		if gotService != serviceID {
			return false, nil
		}
		req := &resource.ResourceFailure{}
		if err := strm.MsgRecv(req); err != nil {
			return true, err
		}
		switch method {
		case "Ping":
			return true, strm.MsgSend(&resource.ResourceFailure{})
		case "Fail":
			return true, errors.New(req.GetMessage())
		}
		return false, nil
	})
}

// callLifecycleTest invokes method on ref with message as its request text.
func callLifecycleTest(ctx context.Context, ref ResourceRef, serviceID, method, message string) error {
	client, err := ref.GetClient()
	if err != nil {
		return err
	}
	return client.ExecCall(ctx, serviceID, method, &resource.ResourceFailure{Message: message}, &resource.ResourceFailure{})
}

// TestResourceRouteFailureDecidesLifetimeByCode checks that handler errors
// carrying former lifecycle phrases, and caller cancellation, leave valid
// handles alive, while a reworded route refusal still retires the handle.
func TestResourceRouteFailureDecidesLifetimeByCode(t *testing.T) {
	// Open one generation against a real server whose refusals are reworded.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server := resource_server.NewResourceServer(lifecycleTestInvoker("test.Root"))
	serverMux := srpc.NewMux()
	rewording := &rewordingResourceServer{ResourceServer: server}
	if err := resource.SRPCRegisterResourceService(serverMux, rewording); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux))))
	client, err := NewClient(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()
	baseline := server.CountTrackedResources()

	// Reference a server-owned root and a client-attached tree root.
	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	attachedID, err := client.AttachResourceTree(ctx, "lifecycle-test", lifecycleTestInvoker("test.Attached"))
	if err != nil {
		t.Fatal(err)
	}
	attachedRef := client.CreateResourceReference(attachedID)
	defer attachedRef.Release()
	handles := []struct {
		name      string
		ref       ResourceRef
		serviceID string
	}{
		{"root", rootRef, "test.Root"},
		{"attached", attachedRef, "test.Attached"},
	}

	// Handler errors and cancellation are call results, not handle lifetime.
	canceledCtx, cancelCall := context.WithCancel(ctx)
	cancelCall()
	for _, handle := range handles {
		for _, phrase := range lifecyclePhrases {
			err := callLifecycleTest(ctx, handle.ref, handle.serviceID, "Fail", phrase)
			if err == nil || err.Error() != phrase {
				t.Fatalf("%s Fail(%q) error = %v", handle.name, phrase, err)
			}
			if errors.Is(err, resource.ErrResourceNotFound) || errors.Is(err, resource.ErrResourceOrClientReleased) {
				t.Fatalf("%s handler error %q matched a lifecycle failure", handle.name, phrase)
			}
		}
		if err := callLifecycleTest(canceledCtx, handle.ref, handle.serviceID, "Ping", ""); err == nil {
			t.Fatalf("%s canceled Ping succeeded", handle.name)
		}
		if err := callLifecycleTest(ctx, handle.ref, handle.serviceID, "Ping", ""); err != nil {
			t.Fatalf("%s Ping after failures: %v", handle.name, err)
		}
	}

	// Detach the attached root and wait until the server returns to baseline.
	if err := client.DetachResource(ctx, attachedID); err != nil {
		t.Fatal(err)
	}
	if got := server.WaitTrackedResourceCount(ctx, baseline); got != baseline {
		t.Fatalf("resource count = %d, want baseline %d", got, baseline)
	}

	// The next route refusal retires the detached handle by its code.
	err = callLifecycleTest(ctx, attachedRef, "test.Attached", "Ping", "")
	if !errors.Is(err, resource.ErrResourceOrClientReleased) || err.Error() != rewordedRouteFailure {
		t.Fatalf("detached Ping error = %v, want reworded released failure", err)
	}
	if _, err := attachedRef.GetClient(); err != resource.ErrResourceOrClientReleased {
		t.Fatalf("detached GetClient error = %v", err)
	}

	// The root survives on the open connection.
	if err := callLifecycleTest(ctx, rootRef, "test.Root", "Ping", ""); err != nil {
		t.Fatalf("root Ping after detach: %v", err)
	}
	select {
	case <-client.Done():
		t.Fatal("client generation retired")
	default:
	}
}
