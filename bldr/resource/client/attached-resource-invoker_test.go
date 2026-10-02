package resource_client

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// TestAttachedResourceInvocationReleasesUnsentChildren exercises real attachment
// publication while cancellation or a handler error prevents an ownership response.
func TestAttachedResourceInvocationReleasesUnsentChildren(t *testing.T) {
	// Exercise each invocation exit without a delivered child ID.
	for _, mode := range []string{"canceled", "handler-error", "send-error", "no-response"} {
		t.Run(mode, func(t *testing.T) {
			// Open a real resource generation and retain its attachment root.
			client, server := newAttachedInvocationTestClient(t)
			defer client.Release()

			// Coordinate publication and completion without scheduling-dependent waits.
			created := make(chan struct{})
			finished := make(chan error, 1)
			released := make(chan struct{}, 2)
			resume := make(chan struct{})
			wantErr := errors.New("child response failed")

			// Serve a root that creates children before choosing its response outcome.
			rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
				// Publish two children through both registration entry points.
				owner, err := resource_server.MustGetResourceClientContext(strm.Context())
				if err != nil {
					return true, err
				}
				if _, err := owner.AddResource(srpc.NewMux(), func() { released <- struct{}{} }); err != nil {
					return true, err
				}
				id, err := owner.AddResourceValue(srpc.NewMux(), "child-value", func() { released <- struct{}{} })
				if err != nil {
					return true, err
				}
				close(created)

				// Hold the response until the test observes both published children.
				if mode == "canceled" {
					<-strm.Context().Done()
					return true, strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: id})
				}
				<-resume
				if mode == "handler-error" {
					return true, wantErr
				}
				if mode == "send-error" {
					return true, strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: id})
				}
				return true, nil
			})
			rootID, err := client.AttachResourceTree(t.Context(), "test-root", rootMux)
			if err != nil {
				t.Fatal(err)
			}
			defer client.DetachResource(t.Context(), rootID)

			// Observe the production invoker's return after its deferred child cleanup.
			sess := client.attach.currentSession()
			sess.router.SetMux(rootID, srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
				// Fail a live response transport after the children have been published.
				if mode == "send-error" {
					strm = &failedResponseStream{Stream: strm, err: wantErr}
				}

				// Run the same publication boundary installed by attachResource.
				ok, err := NewAttachedResourceInvoker(client, rootMux).InvokeMethod(serviceID, methodID, strm)
				finished <- err
				return ok, err
			}))

			// Acquire the attached root's actual resource server RPC route.
			ref := client.CreateResourceReference(rootID)
			defer ref.Release()
			rpc, err := ref.GetClient()
			if err != nil {
				t.Fatal(err)
			}

			// Invoke the root with a cancelable request and a stable resource baseline.
			baseline := server.CountTrackedResources()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			called := make(chan error, 1)
			go func() {
				called <- rpc.ExecCall(ctx, "test.Root", "CreateChild", &resource.ResourceClientInitRequest{}, &resource.ResourceAttachAddAck{})
			}()

			// Observe the created children before allowing the response to proceed.
			<-created
			if got := server.CountTrackedResources(); got != baseline+2 {
				t.Fatalf("published count = %d, want %d", got, baseline+2)
			}

			// End the invocation before any response can transfer the child IDs.
			if mode == "canceled" {
				cancel()
			}
			close(resume)
			invocationErr := <-finished
			if mode == "canceled" && !errors.Is(invocationErr, context.Canceled) {
				t.Fatalf("invocation error = %v, want canceled", invocationErr)
			}
			if (mode == "handler-error" || mode == "send-error") && !errors.Is(invocationErr, wantErr) {
				t.Fatalf("invocation error = %v, want %v", invocationErr, wantErr)
			}
			if err := <-called; mode != "no-response" && err == nil {
				t.Fatal("invocation unexpectedly delivered a response")
			}

			// Require local handles and server registrations to retire on the open connection.
			if got := len(released); got != 2 {
				t.Fatalf("released children = %d, want 2", got)
			}
			if got := server.WaitTrackedResourceCount(t.Context(), baseline); got != baseline {
				t.Fatalf("resource count = %d, want %d", got, baseline)
			}
		})
	}
}

// TestAttachedResourceInvocationRetainsSentChildren proves streaming cancellation
// releases only the next unsent child and preserves the child's attachment lifetime.
func TestAttachedResourceInvocationRetainsSentChildren(t *testing.T) {
	// Open a real generation and coordinate its streamed child publication.
	client, server := newAttachedInvocationTestClient(t)
	defer client.Release()
	created := make(chan struct{})
	finished := make(chan struct{})
	released := make(chan string, 2)

	// Send one child before creating the child whose response will be canceled.
	rootMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Publish the first child with the attachment generation's lifetime.
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		id, err := owner.AddResource(srpc.NewMux(), func() { released <- "sent" })
		if err != nil {
			return true, err
		}
		if err := strm.MsgSend(&resource.ResourceAttachAddAck{ResourceId: id}); err != nil {
			return true, err
		}

		// Hold the next response until the caller cancels the streaming invocation.
		if _, err := owner.AddResource(srpc.NewMux(), func() { released <- "unsent" }); err != nil {
			return true, err
		}
		close(created)
		<-strm.Context().Done()
		return true, strm.Context().Err()
	})
	rootID, err := client.AttachResourceTree(t.Context(), "stream-root", rootMux)
	if err != nil {
		t.Fatal(err)
	}
	defer client.DetachResource(t.Context(), rootID)

	// Observe invocation completion after its pending publication cleanup.
	sess := client.attach.currentSession()
	sess.router.SetMux(rootID, srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// Complete pending cleanup before notifying the test.
		ok, err := NewAttachedResourceInvoker(client, rootMux).InvokeMethod(serviceID, methodID, strm)
		close(finished)
		return ok, err
	}))
	root := client.CreateResourceReference(rootID)
	defer root.Release()
	rpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	baseline := server.CountTrackedResources()

	// Open the actual streaming response with a cancelable invocation context.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := rpc.NewStream(ctx, "test.Root", "WatchChildren", &resource.ResourceClientInitRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Adopt the first child after its ID reaches the caller.
	response := new(resource.ResourceAttachAddAck)
	if err := stream.MsgRecv(response); err != nil {
		t.Fatal(err)
	}
	child := client.CreateResourceReference(response.GetResourceId())
	defer child.Release()

	// Cancel only after both children exist and the first ID reached the caller.
	<-created
	if got := server.CountTrackedResources(); got != baseline+2 {
		t.Fatalf("published count = %d, want %d", got, baseline+2)
	}
	cancel()
	<-finished
	if got := <-released; got != "unsent" || len(released) != 0 {
		t.Fatalf("released child = %q, extra releases = %d", got, len(released))
	}
	if got := server.WaitTrackedResourceCount(t.Context(), baseline+1); got != baseline+1 {
		t.Fatalf("retained child count = %d, want %d", got, baseline+1)
	}

	// Retire the parent while the delivered child remains independently attached.
	root.Release()
	if got := server.WaitTrackedResourceCount(t.Context(), baseline); got != baseline {
		t.Fatalf("count after parent release = %d, want %d", got, baseline)
	}
	select {
	case got := <-released:
		t.Fatalf("parent release retired delivered child %q", got)
	default:
	}

	// Release the delivered child's adopted reference on the still-open connection.
	child.Release()
	if got := <-released; got != "sent" {
		t.Fatalf("explicitly released child = %q, want sent", got)
	}
	if got := server.WaitTrackedResourceCount(t.Context(), baseline-1); got != baseline-1 {
		t.Fatalf("final resource count = %d, want %d", got, baseline-1)
	}
}

// TestAttachedResourceCleanupRegisteredAfterDetach releases a handle whose
// attachment retired before AddResourceValue installed its cleanup callback.
func TestAttachedResourceCleanupRegisteredAfterDetach(t *testing.T) {
	// Remove an attached child before its release callback becomes available.
	sess := newAttachSession(t.Context(), nil, nil, resource.NewRoutedInvoker())
	defer sess.close()
	if err := sess.setMux(42, srpc.NewMux()); err != nil {
		t.Fatal(err)
	}
	sess.releaseAttachedResource(42)

	// Run late cleanup immediately without retaining a stale callback.
	releases := 0
	sess.setRelease(42, func() { releases++ })
	sess.releaseAttachedResource(42)
	if releases != 1 || len(sess.releaseFns) != 0 {
		t.Fatalf("cleanup calls = %d, retained callbacks = %d", releases, len(sess.releaseFns))
	}
}

// newAttachedInvocationTestClient opens an in-process Resource RPC transport.
func newAttachedInvocationTestClient(t *testing.T) (*Client, *resource_server.ResourceServer) {
	// Register the server's root and lifecycle services on a real SRPC pipe.
	t.Helper()
	server := resource_server.NewResourceServer(srpc.NewMux())
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Open the generation whose tracked attachment counts the tests inspect.
	client, err := NewClient(t.Context(), service)
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}
