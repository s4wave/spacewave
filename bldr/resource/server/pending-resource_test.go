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

// TestResourceRPCUndeliveredChildReturnsCountToBaseline exercises registration
// followed by caller cancellation, a failed response send, or a handler error.
func TestResourceRPCUndeliveredChildReturnsCountToBaseline(t *testing.T) {
	for _, mode := range []string{"cancel", "send-failure", "handler-error"} {
		t.Run(mode, func(t *testing.T) {
			// Gate the real resource-returning handler after it registers the child.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			registered := make(chan uint32, 1)
			released := make(chan struct{}, 1)
			resume := make(chan struct{})
			failure := errors.New("response transport failed")
			rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
				// Register the child through the handler's public resource context.
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
				registered <- id

				// Hold response delivery until the test has observed registration.
				var wait <-chan struct{} = resume
				if mode == "cancel" {
					wait = strm.Context().Done()
				}
				select {
				case <-wait:
				case <-strm.Context().Done():
				}
				if mode == "handler-error" {
					return true, errors.New("handler failed after registration")
				}
				return true, strm.MsgSend(&echo.EchoMsg{Body: strconv.FormatUint(uint64(id), 10)})
			})

			// Open a real ResourceClient generation, injecting only the failed transport.
			var sendErr error
			if mode == "send-failure" {
				sendErr = failure
			}
			client, server, rootRPC := newPendingResourceTestClient(t, ctx, rootMux, sendErr)
			defer client.Release()
			baseline := server.CountTrackedResources()

			// Start the gated call with an independently cancelable caller context.
			callCtx, cancelCall := context.WithCancel(ctx)
			defer cancelCall()
			done := make(chan error, 1)
			go func() {
				_, err := echo.NewSRPCEchoerClient(rootRPC).Echo(callCtx, &echo.EchoMsg{})
				done <- err
			}()

			// Require one new resource before triggering the unsuccessful reply.
			select {
			case <-registered:
			case <-ctx.Done():
				t.Fatal("handler did not register its child")
			}
			if got := server.CountTrackedResources(); got != baseline+1 {
				t.Fatalf("registered count = %d, want %d", got, baseline+1)
			}
			if mode == "cancel" {
				cancelCall()
			}
			close(resume)

			// Require a failed call and release while the client generation stays open.
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("undelivered response returned success")
				}
			case <-ctx.Done():
				t.Fatal("resource call did not finish")
			}
			if got := server.WaitTrackedResourceCount(ctx, baseline); got != baseline {
				t.Fatalf("resource count = %d, want baseline %d", got, baseline)
			}
			select {
			case <-released:
			case <-ctx.Done():
				t.Fatal("resource release callback did not run")
			}
			select {
			case <-client.Done():
				t.Fatal("client generation retired during cleanup")
			default:
			}
		})
	}
}

// TestResourceRPCDeliveredChildSurvivesCallCancellation checks response transfer
// before adoption and independent lifetime after successful adoption.
func TestResourceRPCDeliveredChildSurvivesCallCancellation(t *testing.T) {
	// Keep the resource-returning handler alive after its successful response.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	released := make(chan struct{}, 1)
	finished := make(chan struct{})
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Register a child with a real echo service and publish its resource ID.
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		childMux := srpc.NewMux()
		if err := echo.SRPCRegisterEchoer(childMux, echo.NewEchoServer(childMux)); err != nil {
			return true, err
		}
		id, err := owner.AddResource(childMux, func() { released <- struct{}{} })
		if err != nil {
			return true, err
		}
		if err := strm.MsgSend(&echo.EchoMsg{Body: strconv.FormatUint(uint64(id), 10)}); err != nil {
			return true, err
		}

		// Observe the post-response cancellation before invocation cleanup runs.
		<-strm.Context().Done()
		close(finished)
		return true, strm.Context().Err()
	})
	client, server, rootRPC := newPendingResourceTestClient(t, ctx, rootMux, nil)
	defer client.Release()
	baseline := server.CountTrackedResources()

	// Receive the resource ID and let the call finish before adopting it.
	response, err := echo.NewSRPCEchoerClient(rootRPC).Echo(ctx, &echo.EchoMsg{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseUint(response.GetBody(), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("handler did not observe call completion")
	}
	child := client.CreateResourceReference(uint32(id))
	defer child.Release()

	// Invoke the adopted child through the ordered ResourceClient control stream.
	childRPC, err := child.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	got, err := echo.NewSRPCEchoerClient(childRPC).Echo(ctx, &echo.EchoMsg{Body: "adopted child"})
	if err != nil || got.GetBody() != "adopted child" {
		t.Fatalf("adopted child response = %v, error = %v", got, err)
	}
	if count := server.CountTrackedResources(); count != baseline+1 {
		t.Fatalf("adopted count = %d, want %d", count, baseline+1)
	}
	select {
	case <-released:
		t.Fatal("delivered child released by invocation cleanup")
	default:
	}

	// Release the adopted child without disconnecting the client.
	child.Release()
	if count := server.WaitTrackedResourceCount(ctx, baseline); count != baseline {
		t.Fatalf("released count = %d, want baseline %d", count, baseline)
	}
}

// TestResourceRPCStreamingCancellationReleasesOnlyUnpublishedChildren checks
// that a later canceled send preserves the child delivered by an earlier send.
func TestResourceRPCStreamingCancellationReleasesOnlyUnpublishedChildren(t *testing.T) {
	// Publish one streamed child, then gate the second child before its response.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	registered := make(chan struct{})
	released := make(chan string, 2)
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Register the first child and deliver its ID to the streaming caller.
		if err := strm.MsgRecv(&echo.EchoMsg{}); err != nil {
			return true, err
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		id, err := owner.AddResource(srpc.NewMux(), func() { released <- "sent" })
		if err != nil {
			return true, err
		}
		if err := strm.MsgSend(&echo.EchoMsg{Body: strconv.FormatUint(uint64(id), 10)}); err != nil {
			return true, err
		}

		// Register a second child whose response cannot reach the canceled caller.
		if _, err := owner.AddResourceValue(srpc.NewMux(), "unsent child", func() { released <- "unsent" }); err != nil {
			return true, err
		}
		close(registered)
		<-strm.Context().Done()
		return true, strm.Context().Err()
	})
	client, server, rpc := newPendingResourceTestClient(t, ctx, rootMux, nil)
	defer client.Release()
	baseline := server.CountTrackedResources()

	// Receive and adopt the first child through the real ResourceClient interface.
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	stream, err := echo.NewSRPCEchoerClient(rpc).EchoServerStream(callCtx, &echo.EchoMsg{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Decode the first child's ID and retain its independently adopted reference.
	response, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseUint(response.GetBody(), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	child := client.CreateResourceReference(uint32(id))
	defer child.Release()

	// Cancel after both children exist and retain the independently published child.
	select {
	case <-registered:
	case <-ctx.Done():
		t.Fatal("second streamed child was not registered")
	}
	if got := server.CountTrackedResources(); got != baseline+2 {
		t.Fatalf("streamed resource count = %d, want %d", got, baseline+2)
	}
	cancelCall()
	if got := server.WaitTrackedResourceCount(ctx, baseline+1); got != baseline+1 {
		t.Fatalf("remaining count = %d, want %d", got, baseline+1)
	}
	select {
	case label := <-released:
		if label != "unsent" {
			t.Fatalf("released child = %q, want unsent", label)
		}
	case <-ctx.Done():
		t.Fatal("unsent streamed child was not released")
	}

	// Release the adopted streamed child on the still-open connection.
	child.Release()
	if got := server.WaitTrackedResourceCount(ctx, baseline); got != baseline {
		t.Fatalf("final count = %d, want baseline %d", got, baseline)
	}
	select {
	case label := <-released:
		if label != "sent" {
			t.Fatalf("released child = %q, want sent", label)
		}
	case <-ctx.Done():
		t.Fatal("adopted streamed child was not released")
	}
}

// newPendingResourceTestClient opens a real in-process Resource RPC connection.
func newPendingResourceTestClient(t *testing.T, ctx context.Context, root srpc.Invoker, sendErr error) (*resource_client.Client, *resource_server.ResourceServer, srpc.Client) {
	// Register the server, optionally refusing only ResourceRpc response data.
	t.Helper()
	server := resource_server.NewResourceServer(root)
	mux := srpc.NewMux()
	var impl resource.SRPCResourceServiceServer = server
	if sendErr != nil {
		impl = &failedResourceRPCServer{ResourceServer: server, err: sendErr}
	}
	if err := resource.SRPCRegisterResourceService(mux, impl); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Retain the generation's root reference for the test's calls.
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Release)
	rootRef := client.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rpc, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	return client, server, rpc
}
