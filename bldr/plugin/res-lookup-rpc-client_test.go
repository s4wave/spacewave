package bldr_plugin

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// TestLookupRpcClientReleasesBeforeReplacement checks client invalidation and
// directive cancellation through the same retained-client lifetime.
func TestLookupRpcClientReleasesBeforeReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)

	// Each acquisition must follow release of the preceding client.
	var acquisitions int32
	var releases atomic.Int32
	invalidators := make(chan func(), 1)
	handler := lookupClientHandlerFunc(func(_ context.Context, invalidated func()) (srpc.Client, func(), error) {
		if releases.Load() != acquisitions {
			return nil, nil, errors.New("replacement acquired before previous client release")
		}
		acquisitions++
		invalidators <- invalidated
		return &testForwardingClient{}, func() { releases.Add(1) }, nil
	})
	resolver := NewLookupRpcClientResolver(handler, "materializer", "")
	done := make(chan error, 1)
	go func() { done <- resolver.Resolve(ctx, stubResolverHandler{}) }()

	// Invalidate the published client and observe a replacement publication.
	first, err := resolver.GetRpcClientCtr().WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	(<-invalidators)()
	second, err := resolver.GetRpcClientCtr().WaitValueChange(ctx, first, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if second == nil {
		if _, err := resolver.GetRpcClientCtr().WaitValue(ctx, nil); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Cancellation must release the replacement and clear the published client.
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
	if got := releases.Load(); got != 2 {
		t.Fatalf("released %d clients, want 2", got)
	}
	if resolver.GetRpcClientCtr().GetValue() != nil {
		t.Fatal("canceled resolver still publishes a client")
	}
}

// TestLookupRpcClientHostNeedsNoRelease verifies that the host-owned client can
// be withdrawn without a caller release function.
func TestLookupRpcClientHostNeedsNoRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)
	resolver := NewLookupRpcClientResolver(&nilReleaseClientHandler{
		client: &testForwardingClient{},
	}, "", "")
	done := make(chan error, 1)
	go func() { done <- resolver.Resolve(ctx, stubResolverHandler{}) }()

	// Wait for a real publication before canceling the host lookup.
	if _, err := resolver.GetRpcClientCtr().WaitValue(ctx, nil); err != nil {
		t.Fatal(err.Error())
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
	if resolver.GetRpcClientCtr().GetValue() != nil {
		t.Fatal("canceled host lookup still publishes a client")
	}
}

// lookupClientHandlerFunc supplies clients to both plugin lookup paths.
type lookupClientHandlerFunc func(context.Context, func()) (srpc.Client, func(), error)

// WaitPluginHostClient delegates the host lookup to the test function.
func (f lookupClientHandlerFunc) WaitPluginHostClient(ctx context.Context, invalidated func()) (srpc.Client, func(), error) {
	return f(ctx, invalidated)
}

// WaitPluginClient delegates the plugin lookup to the test function.
func (f lookupClientHandlerFunc) WaitPluginClient(ctx context.Context, invalidated func(), _ string) (srpc.Client, func(), error) {
	return f(ctx, invalidated)
}

// lookupClientHandlerFunc implements the retained-client lookup contract.
var _ LookupRpcClientHandler = lookupClientHandlerFunc(nil)
