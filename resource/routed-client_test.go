package resource

import (
	"context"
	strconv "strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

// recordingInvoker records InvokeMethod calls and returns configurable results.
type recordingInvoker struct {
	mu       sync.Mutex
	calls    []invokerCall
	retFound bool
	retErr   error
}

type invokerCall struct {
	serviceID string
	methodID  string
}

func (r *recordingInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	r.mu.Lock()
	r.calls = append(r.calls, invokerCall{serviceID: serviceID, methodID: methodID})
	r.mu.Unlock()
	return r.retFound, r.retErr
}

func (r *recordingInvoker) getCalls() []invokerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]invokerCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// recordingClient records ExecCall and NewStream calls with their service/method IDs.
type recordingClient struct {
	mu      sync.Mutex
	calls   []clientCall
	retErr  error
	retStrm srpc.Stream
	strmErr error
}

type clientCall struct {
	op      string
	service string
	method  string
}

func (c *recordingClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	c.mu.Lock()
	c.calls = append(c.calls, clientCall{op: "exec", service: service, method: method})
	c.mu.Unlock()
	return c.retErr
}

func (c *recordingClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	c.mu.Lock()
	c.calls = append(c.calls, clientCall{op: "stream", service: service, method: method})
	c.mu.Unlock()
	return c.retStrm, c.strmErr
}

func (c *recordingClient) getCalls() []clientCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]clientCall, len(c.calls))
	copy(out, c.calls)
	return out
}

// --- RoutedInvoker tests ---

func TestRoutedInvoker_HappyPath(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(42, inv)

	found, err := ri.InvokeMethod("42/foo.Service", "Bar", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}

	calls := inv.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].serviceID != "foo.Service" {
		t.Fatalf("got serviceID %q, want %q", calls[0].serviceID, "foo.Service")
	}
	if calls[0].methodID != "Bar" {
		t.Fatalf("got methodID %q, want %q", calls[0].methodID, "Bar")
	}
}

func TestRoutedInvoker_NoSlashReturnsFalse(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(1, inv)

	found, err := ri.InvokeMethod("no-slash-here", "Method", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for service ID without slash")
	}
	if len(inv.getCalls()) != 0 {
		t.Fatal("invoker should not have been called")
	}
}

func TestRoutedInvoker_NonNumericPrefixReturnsFalse(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(1, inv)

	found, err := ri.InvokeMethod("abc/foo.Service", "Method", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for non-numeric prefix")
	}
	if len(inv.getCalls()) != 0 {
		t.Fatal("invoker should not have been called")
	}
}

func TestRoutedInvoker_MuxNotFoundReturnsError(t *testing.T) {
	ri := NewRoutedInvoker()

	found, err := ri.InvokeMethod("99/foo.Service", "Method", nil)
	if found {
		t.Fatal("expected found=false for missing mux")
	}
	if err != ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, ErrResourceNotFound)
	}
}

func TestRoutedInvoker_SetMuxRegisters(t *testing.T) {
	ri := NewRoutedInvoker()

	// Before SetMux, the mux should not be found.
	found, err := ri.InvokeMethod("7/svc", "m", nil)
	if found {
		t.Fatal("expected found=false before SetMux")
	}
	if err != ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, ErrResourceNotFound)
	}

	inv := &recordingInvoker{retFound: true}
	ri.SetMux(7, inv)

	found, err = ri.InvokeMethod("7/svc", "m", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after SetMux")
	}
	if len(inv.getCalls()) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inv.getCalls()))
	}
}

func TestRoutedInvoker_RemoveMux(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(5, inv)

	// Verify it works before removal.
	found, err := ri.InvokeMethod("5/svc", "m", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true before RemoveMux")
	}

	ri.RemoveMux(5)

	found, err = ri.InvokeMethod("5/svc", "m", nil)
	if found {
		t.Fatal("expected found=false after RemoveMux")
	}
	if err != ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, ErrResourceNotFound)
	}
}

func TestRoutedInvoker_SetMuxReplaces(t *testing.T) {
	ri := NewRoutedInvoker()
	inv1 := &recordingInvoker{retFound: true}
	inv2 := &recordingInvoker{retFound: true}

	ri.SetMux(3, inv1)
	ri.SetMux(3, inv2)

	found, err := ri.InvokeMethod("3/svc", "m", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}

	if len(inv1.getCalls()) != 0 {
		t.Fatal("old invoker should not have been called")
	}
	if len(inv2.getCalls()) != 1 {
		t.Fatalf("new invoker expected 1 call, got %d", len(inv2.getCalls()))
	}
}

func TestRoutedInvoker_MultipleMuxes(t *testing.T) {
	ri := NewRoutedInvoker()
	inv1 := &recordingInvoker{retFound: true}
	inv2 := &recordingInvoker{retFound: true}

	ri.SetMux(10, inv1)
	ri.SetMux(20, inv2)

	found, err := ri.InvokeMethod("10/svcA", "methodA", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for mux 10")
	}

	found, err = ri.InvokeMethod("20/svcB", "methodB", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for mux 20")
	}

	calls1 := inv1.getCalls()
	calls2 := inv2.getCalls()
	if len(calls1) != 1 || calls1[0].serviceID != "svcA" {
		t.Fatalf("mux 10: got calls %+v, want serviceID=svcA", calls1)
	}
	if len(calls2) != 1 || calls2[0].serviceID != "svcB" {
		t.Fatalf("mux 20: got calls %+v, want serviceID=svcB", calls2)
	}
}

func TestRoutedInvoker_PropagatesMuxError(t *testing.T) {
	ri := NewRoutedInvoker()
	muxErr := ErrInvalidResourceID
	inv := &recordingInvoker{retFound: true, retErr: muxErr}
	ri.SetMux(1, inv)

	found, err := ri.InvokeMethod("1/svc", "m", nil)
	if !found {
		t.Fatal("expected found=true (propagated from mux)")
	}
	if err != muxErr {
		t.Fatalf("got error %v, want %v", err, muxErr)
	}
}

func TestRoutedInvoker_EmptyServiceAfterSlash(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(1, inv)

	found, err := ri.InvokeMethod("1/", "m", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	calls := inv.getCalls()
	if len(calls) != 1 || calls[0].serviceID != "" {
		t.Fatalf("expected empty serviceID, got %q", calls[0].serviceID)
	}
}

func TestRoutedInvoker_NestedSlash(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(1, inv)

	found, err := ri.InvokeMethod("1/foo/bar", "m", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	calls := inv.getCalls()
	if len(calls) != 1 || calls[0].serviceID != "foo/bar" {
		t.Fatalf("expected serviceID %q, got %q", "foo/bar", calls[0].serviceID)
	}
}

func TestRoutedInvoker_ConcurrentSetRemove(t *testing.T) {
	ri := NewRoutedInvoker()
	var wg sync.WaitGroup
	for i := range 100 {
		id := uint32(i)
		wg.Add(2)
		go func() {
			defer wg.Done()
			ri.SetMux(id, &recordingInvoker{retFound: true})
		}()
		go func() {
			defer wg.Done()
			ri.RemoveMux(id)
		}()
	}
	wg.Wait()
}

func TestRoutedInvoker_ConcurrentInvoke(t *testing.T) {
	ri := NewRoutedInvoker()
	for i := range 10 {
		ri.SetMux(uint32(i), &recordingInvoker{retFound: true})
	}

	var wg sync.WaitGroup
	for i := range 10 {
		id := uint32(i)
		wg.Go(func() {
			svc := strconv.FormatUint(uint64(id), 10) + "/svc"
			ri.InvokeMethod(svc, "m", nil)
		})
	}
	wg.Wait()
}

// --- NewRoutedClient tests ---

func TestRoutedClient_ExecCallPrependsPrefix(t *testing.T) {
	rec := &recordingClient{}
	rc := NewRoutedClient(rec, 42)

	err := rc.ExecCall(context.Background(), "foo.Service", "Bar", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := rec.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].op != "exec" {
		t.Fatalf("expected op=exec, got %q", calls[0].op)
	}
	if calls[0].service != "42/foo.Service" {
		t.Fatalf("got service %q, want %q", calls[0].service, "42/foo.Service")
	}
	if calls[0].method != "Bar" {
		t.Fatalf("got method %q, want %q", calls[0].method, "Bar")
	}
}

func TestRoutedClient_NewStreamPrependsPrefix(t *testing.T) {
	rec := &recordingClient{}
	rc := NewRoutedClient(rec, 7)

	_, err := rc.NewStream(context.Background(), "my.Svc", "DoThing", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := rec.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].op != "stream" {
		t.Fatalf("expected op=stream, got %q", calls[0].op)
	}
	if calls[0].service != "7/my.Svc" {
		t.Fatalf("got service %q, want %q", calls[0].service, "7/my.Svc")
	}
	if calls[0].method != "DoThing" {
		t.Fatalf("got method %q, want %q", calls[0].method, "DoThing")
	}
}

func TestRoutedClient_MethodPassesThrough(t *testing.T) {
	rec := &recordingClient{}
	rc := NewRoutedClient(rec, 1)

	rc.ExecCall(context.Background(), "svc", "MyMethod", nil, nil)
	rc.NewStream(context.Background(), "svc", "OtherMethod", nil)

	calls := rec.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].method != "MyMethod" {
		t.Fatalf("exec: got method %q, want %q", calls[0].method, "MyMethod")
	}
	if calls[1].method != "OtherMethod" {
		t.Fatalf("stream: got method %q, want %q", calls[1].method, "OtherMethod")
	}
}

func TestRoutedClient_ZeroResourceID(t *testing.T) {
	rec := &recordingClient{}
	rc := NewRoutedClient(rec, 0)

	rc.ExecCall(context.Background(), "svc", "m", nil, nil)

	calls := rec.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].service != "0/svc" {
		t.Fatalf("got service %q, want %q", calls[0].service, "0/svc")
	}
}

func TestRoutedClient_LargeResourceID(t *testing.T) {
	rec := &recordingClient{}
	rc := NewRoutedClient(rec, 4294967295) // max uint32

	rc.ExecCall(context.Background(), "svc", "m", nil, nil)

	calls := rec.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].service != "4294967295/svc" {
		t.Fatalf("got service %q, want %q", calls[0].service, "4294967295/svc")
	}
}

func TestRoutedClient_PropagatesExecError(t *testing.T) {
	rec := &recordingClient{retErr: ErrClientReleased}
	rc := NewRoutedClient(rec, 1)

	err := rc.ExecCall(context.Background(), "svc", "m", nil, nil)
	if err != ErrClientReleased {
		t.Fatalf("got error %v, want %v", err, ErrClientReleased)
	}
}

func TestRoutedClient_PropagatesStreamError(t *testing.T) {
	rec := &recordingClient{strmErr: ErrClientReleased}
	rc := NewRoutedClient(rec, 1)

	_, err := rc.NewStream(context.Background(), "svc", "m", nil)
	if err != ErrClientReleased {
		t.Fatalf("got error %v, want %v", err, ErrClientReleased)
	}
}

// --- Roundtrip: NewRoutedClient -> RoutedInvoker ---

func TestRoutedRoundtrip(t *testing.T) {
	ri := NewRoutedInvoker()
	inv := &recordingInvoker{retFound: true}
	ri.SetMux(99, inv)

	// Simulate what NewRoutedClient produces being consumed by RoutedInvoker.
	// The client prefixes "99/" to the service ID; the invoker strips it.
	prefixed := "99/" + "my.Package.Service"
	found, err := ri.InvokeMethod(prefixed, "Execute", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	calls := inv.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].serviceID != "my.Package.Service" {
		t.Fatalf("got serviceID %q, want %q", calls[0].serviceID, "my.Package.Service")
	}
	if calls[0].methodID != "Execute" {
		t.Fatalf("got methodID %q, want %q", calls[0].methodID, "Execute")
	}
}
