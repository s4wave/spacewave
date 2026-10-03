package web_runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

func TestWebWorkerHostInvokerWrapsInvokeErrorWithTarget(t *testing.T) {
	// Create a worker invoker that returns an error for a known RPC target.
	invokeErr := errors.New("context canceled")
	invoker := &webWorkerHostInvoker{
		webWorkerID: "plugin/spacewave-notes",
		invoker: srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
			// Verify the resource service and method forwarded to the worker.
			if serviceID != "bldr.resource.ResourceService" {
				t.Fatalf("unexpected service id: %s", serviceID)
			}
			if methodID != "ResourceClient" {
				t.Fatalf("unexpected method id: %s", methodID)
			}

			return true, invokeErr
		}),
	}

	// Invoke the worker RPC target through the error-wrapping adapter.
	ok, err := invoker.InvokeMethod("bldr.resource.ResourceService", "ResourceClient", nil)

	// Verify that the error retains its cause and the worker RPC target.
	if !ok {
		t.Fatal("expected method to be handled")
	}
	if !errors.Is(err, invokeErr) {
		t.Fatalf("expected wrapped invoke error, got %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "web-worker/plugin/spacewave-notes bldr.resource.ResourceService/ResourceClient") {
		t.Fatalf("expected web worker target in error, got %q", msg)
	}
}

func TestWebWorkerHostInvokerPreservesSuccessfulInvoke(t *testing.T) {
	invoker := &webWorkerHostInvoker{
		webWorkerID: "plugin/spacewave-notes",
		invoker: srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
			return true, nil
		}),
	}

	ok, err := invoker.InvokeMethod("service", "method", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected method to be handled")
	}
}
