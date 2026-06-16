package bldr_plugin

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

func TestResolveLookupRpcServiceUsesPluginServiceIDPrefix(t *testing.T) {
	serviceID := PluginServiceID("spacewave-notes", "resource.ResourceService")
	resolver, err := ResolveLookupRpcService(
		t.Context(),
		bifrost_rpc.NewLookupRpcService(serviceID, ""),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := resolver.(*LookupRpcServiceResolver)
	if !ok {
		t.Fatalf("expected LookupRpcServiceResolver, got %T", resolver)
	}
	if got.pluginID != "spacewave-notes" {
		t.Fatalf("expected plugin id spacewave-notes, got %q", got.pluginID)
	}
	if got.stripServiceIDPrefix != "plugin/spacewave-notes/" {
		t.Fatalf("expected service prefix strip, got %q", got.stripServiceIDPrefix)
	}
}

func TestResolveLookupRpcServiceDoesNotRouteRequesterServerID(t *testing.T) {
	resolver, err := ResolveLookupRpcService(
		t.Context(),
		bifrost_rpc.NewLookupRpcService(
			"resource.ResourceService",
			PluginServerID("spacewave-notes", ""),
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolver != nil {
		t.Fatalf("expected plain service lookup with plugin requester server ID to be ignored, got %T", resolver)
	}
}

func TestResolveLookupRpcServiceHandlesNilReleaseFunc(t *testing.T) {
	// Regression: WaitPluginClient/WaitPluginHostClient return a nil release func
	// on the loopback and plugin-host client paths. Resolve must not defer a nil
	// func call, which previously panicked (nil func value, pc=0x0) when the
	// deferred ran and crashed the whole plugin process.
	resolver := NewLookupRpcServiceResolver(
		&nilReleaseClientHandler{client: &testForwardingClient{}},
		"mercury-core",
		"plugin/mercury-core/",
	)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := resolver.Resolve(ctx, stubResolverHandler{}); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestClientForwardingInvokerReturnsWhenServerCompletesBeforeCaller(t *testing.T) {
	outgoing := newTestForwardingStream(t.Context(), [][]byte{[]byte("server-response")})
	local := &testForwardingLocalStream{
		ctx: t.Context(),
		recv: func(msg *srpc.RawMessage) error {
			<-outgoing.closed
			msg.SetData([]byte("late-client-message"))
			return nil
		},
	}
	client := &testForwardingClient{stream: outgoing}

	ok, err := newClientForwardingInvoker(client, "plugin/test/").
		InvokeMethod("plugin/test/resource.ResourceService", "ResourceRpc", local)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected forwarding invoker to handle the method")
	}
	if client.serviceID != "resource.ResourceService" || client.methodID != "ResourceRpc" {
		t.Fatalf("unexpected forwarded target: %s/%s", client.serviceID, client.methodID)
	}
	if got := local.sentStrings(); len(got) != 1 || got[0] != "server-response" {
		t.Fatalf("expected server response to reach caller, got %q", got)
	}
}

func TestClientForwardingInvokerPreservesServerStreamingHalfClose(t *testing.T) {
	outgoing := newTestForwardingStream(t.Context(), [][]byte{[]byte("stream-init")})
	outgoing.waitCloseSendBeforeRecv = true

	var sent bool
	local := &testForwardingLocalStream{
		ctx: t.Context(),
		recv: func(msg *srpc.RawMessage) error {
			if sent {
				return io.EOF
			}
			sent = true
			msg.SetData([]byte("stream-request"))
			return nil
		},
	}

	ok, err := newClientForwardingInvoker(&testForwardingClient{stream: outgoing}, "").
		InvokeMethod("resource.ResourceService", "ResourceClient", local)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected forwarding invoker to handle the method")
	}
	if got := outgoing.sentStrings(); len(got) != 1 || got[0] != "stream-request" {
		t.Fatalf("expected caller request to reach server, got %q", got)
	}
	if !outgoing.closeSendCalled() {
		t.Fatal("expected caller EOF to close the outgoing send side")
	}
	if got := local.sentStrings(); len(got) != 1 || got[0] != "stream-init" {
		t.Fatalf("expected server-streaming response to reach caller, got %q", got)
	}
}

type testForwardingClient struct {
	stream    *testForwardingStream
	serviceID string
	methodID  string
}

func (c *testForwardingClient) ExecCall(context.Context, string, string, srpc.Message, srpc.Message) error {
	return errors.New("unexpected ExecCall")
}

func (c *testForwardingClient) NewStream(
	ctx context.Context,
	serviceID string,
	methodID string,
	firstMsg srpc.Message,
) (srpc.Stream, error) {
	if firstMsg != nil {
		return nil, errors.New("unexpected first message")
	}
	c.serviceID = serviceID
	c.methodID = methodID
	c.stream.ctx = ctx
	return c.stream, nil
}

type testForwardingStream struct {
	ctx context.Context

	mu                      sync.Mutex
	responses               [][]byte
	recvIdx                 int
	sent                    [][]byte
	closeSend               bool
	waitCloseSendBeforeRecv bool

	closeSendCh chan struct{}
	closed      chan struct{}
	closeSendDo sync.Once
	closeDo     sync.Once
}

func newTestForwardingStream(ctx context.Context, responses [][]byte) *testForwardingStream {
	return &testForwardingStream{
		ctx:         ctx,
		responses:   responses,
		closeSendCh: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (s *testForwardingStream) Context() context.Context {
	return s.ctx
}

func (s *testForwardingStream) MsgSend(msg srpc.Message) error {
	select {
	case <-s.closed:
		return srpc.ErrCompleted
	default:
	}

	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.sent = append(s.sent, append([]byte(nil), data...))
	s.mu.Unlock()
	return nil
}

func (s *testForwardingStream) MsgRecv(msg srpc.Message) error {
	s.mu.Lock()
	wait := s.waitCloseSendBeforeRecv && s.recvIdx == 0
	s.mu.Unlock()
	if wait {
		select {
		case <-s.closeSendCh:
		case <-s.closed:
			return context.Canceled
		case <-s.ctx.Done():
			return context.Canceled
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recvIdx >= len(s.responses) {
		return io.EOF
	}
	data := append([]byte(nil), s.responses[s.recvIdx]...)
	s.recvIdx++

	raw, ok := msg.(*srpc.RawMessage)
	if !ok {
		return errors.New("unexpected message type")
	}
	raw.SetData(data)
	return nil
}

func (s *testForwardingStream) CloseSend() error {
	s.closeSendDo.Do(func() {
		s.mu.Lock()
		s.closeSend = true
		s.mu.Unlock()
		close(s.closeSendCh)
	})
	return nil
}

func (s *testForwardingStream) Close() error {
	s.closeDo.Do(func() {
		close(s.closed)
	})
	return nil
}

func (s *testForwardingStream) sentStrings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.sent))
	for _, data := range s.sent {
		out = append(out, string(data))
	}
	return out
}

func (s *testForwardingStream) closeSendCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeSend
}

type testForwardingLocalStream struct {
	ctx  context.Context
	recv func(*srpc.RawMessage) error

	mu   sync.Mutex
	sent [][]byte
}

func (s *testForwardingLocalStream) Context() context.Context {
	return s.ctx
}

func (s *testForwardingLocalStream) MsgSend(msg srpc.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.sent = append(s.sent, append([]byte(nil), data...))
	s.mu.Unlock()
	return nil
}

func (s *testForwardingLocalStream) MsgRecv(msg srpc.Message) error {
	raw, ok := msg.(*srpc.RawMessage)
	if !ok {
		return errors.New("unexpected message type")
	}
	return s.recv(raw)
}

func (s *testForwardingLocalStream) CloseSend() error {
	return nil
}

func (s *testForwardingLocalStream) Close() error {
	return nil
}

func (s *testForwardingLocalStream) sentStrings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.sent))
	for _, data := range s.sent {
		out = append(out, string(data))
	}
	return out
}

// nilReleaseClientHandler returns a non-nil client and a nil release func,
// matching the loopback and plugin-host client paths.
type nilReleaseClientHandler struct {
	client srpc.Client
}

func (h *nilReleaseClientHandler) WaitPluginHostClient(context.Context, func()) (srpc.Client, func(), error) {
	return h.client, nil, nil
}

func (h *nilReleaseClientHandler) WaitPluginClient(context.Context, func(), string) (srpc.Client, func(), error) {
	return h.client, nil, nil
}

// stubResolverHandler is a no-op directive.ResolverHandler for resolver tests.
type stubResolverHandler struct{}

func (stubResolverHandler) AddValue(directive.Value) (uint32, bool)       { return 1, true }
func (stubResolverHandler) RemoveValue(uint32) (directive.Value, bool)    { return nil, true }
func (stubResolverHandler) CountValues(bool) int                          { return 0 }
func (stubResolverHandler) ClearValues() []uint32                         { return nil }
func (stubResolverHandler) MarkIdle(bool)                                 {}
func (stubResolverHandler) AddValueRemovedCallback(uint32, func()) func() { return func() {} }
func (stubResolverHandler) AddResolverRemovedCallback(func()) func()      { return func() {} }
func (stubResolverHandler) AddResolver(directive.Resolver, func()) func() { return func() {} }

var (
	_ srpc.Client               = (*testForwardingClient)(nil)
	_ srpc.Stream               = (*testForwardingStream)(nil)
	_ srpc.Stream               = (*testForwardingLocalStream)(nil)
	_ LookupRpcClientHandler    = (*nilReleaseClientHandler)(nil)
	_ directive.ResolverHandler = stubResolverHandler{}
)
