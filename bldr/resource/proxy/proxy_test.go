package resource_proxy_test

import (
	"context"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_proxy "github.com/s4wave/spacewave/bldr/resource/proxy"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// childService names the service whose Open method adds a child resource
// beneath the resource it is called on.
const childService = "test.Children"

// rules gives the echo methods one rule each.
var rules = resource_proxy.Rules{
	echo.SRPCEchoerServiceID: {
		"Echo":             resource_proxy.Retry,
		"EchoServerStream": resource_proxy.Reopen,
		"DoNothing":        resource_proxy.Once,
		"EchoClientStream": resource_proxy.Replay,
	},
	childService: {
		"Open": resource_proxy.Open,
	},
	viewService: {
		"Watch": resource_proxy.Resume,
	},
}

// TestProxyCarriesCallsAcrossRestart runs a resource client through the proxy
// while its upstream restarts twice, and the generation lives on. Across the
// first restart a held Retry call is answered by the new upstream, a Once call
// in flight fails with a reset, a Reopen stream continues without repeating
// its state, and the Replay call reaches the new upstream first. Across the
// second, the children the client opened, one nested in another, open again
// after the Replay call and keep their IDs, while the one the new upstream
// refuses is released. An upstream release reaches the client by the proxy's
// ID. A unary call past the held call cap is reset at once.
func TestProxyCarriesCallsAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// Serve a client through the proxy to the first upstream.
	var current atomic.Pointer[backend]
	current.Store(newBackend(1))
	var capped atomic.Int32
	held := resource_proxy.NewLimit(2, func() { capped.Add(1) })
	proxy := resource_proxy.NewProxy(ctx, rules, func(context.Context) (resource.SRPCResourceServiceClient, error) {
		b := current.Load()
		if b.dead.Load() {
			return nil, errors.New("upstream down")
		}
		return resource.NewSRPCResourceServiceClient(srpc.NewClient(b.open)), nil
	}, held)
	mux := srpc.NewMux()
	if err := resource.SRPCRegisterResourceService(mux, proxy); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Release()
	root := client.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	echoer := echo.NewSRPCEchoerClient(rootClient)

	// Bind state to the connection with the Replay call.
	bindStream, err := echoer.EchoClientStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := bindStream.Send(&echo.EchoMsg{Body: "device"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bindStream.CloseAndRecv(); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Open a watch and take its state.
	watch, err := echoer.EchoServerStream(ctx, &echo.EchoMsg{Body: "state"})
	if err != nil {
		t.Fatal(err)
	}
	if msg, err := watch.Recv(); err != nil || msg.GetBody() != "state" {
		t.Fatalf("first watch message: %v %v", msg, err)
	}

	// Leave a Retry and a Once call in flight on the first upstream.
	first := current.Load()
	retried := make(chan error, 1)
	go func() {
		msg, err := echoer.Echo(ctx, &echo.EchoMsg{Body: "hold"})
		if err == nil && msg.GetBody() != "hold" {
			err = errors.Errorf("echo answered %q", msg.GetBody())
		}
		retried <- err
	}()
	once := make(chan error, 1)
	go func() {
		_, err := echoer.DoNothing(ctx, &emptypb.Empty{})
		once <- err
	}()
	// An unclassified stream resets rather than replaying after a loss.
	unruled, err := echoer.EchoBidiStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() { ended <- unruled.MsgRecv(&echo.EchoMsg{}) }()
	first.waitCalls(t, ctx, "Echo", "DoNothing", "EchoBidiStream")

	// The two held calls fill the cap, so a third is reset without waiting.
	if _, err := echoer.Echo(ctx, &echo.EchoMsg{Body: "over"}); !errors.Is(err, srpc.ErrReset) || capped.Load() != 1 {
		t.Fatalf("call past the cap: got %v after %d resets, want one reset", err, capped.Load())
	}

	// Restart the upstream.
	first.kill()
	second := newBackend(2)
	current.Store(second)

	// The Retry call is answered and the Once call is reset.
	if err := <-retried; err != nil {
		t.Fatalf("retried echo: %v", err)
	}
	if err := <-ended; !errors.Is(err, srpc.ErrReset) {
		t.Fatalf("unclassified stream: %v", err)
	}
	if err := <-once; !errors.Is(err, srpc.ErrReset) {
		t.Fatalf("once call: got %v, want reset", err)
	}

	// The watch continues with the new upstream's change only.
	if msg, err := watch.Recv(); err != nil || msg.GetBody() != "changed" {
		t.Fatalf("reopened watch message: %v %v", msg, err)
	}

	// The new upstream saw the Replay call before any other.
	if calls := second.callList(); len(calls) == 0 || calls[0] != "EchoClientStream" {
		t.Fatalf("second upstream calls: %v", calls)
	}
	// Open a child, a child of it, and a child the next upstream refuses.
	device := openChild(t, ctx, client, rootClient, "device")
	defer device.Release()
	deviceClient, err := device.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	host := openChild(t, ctx, client, deviceClient, "host")
	defer host.Release()
	hostClient, err := host.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	gone := openChild(t, ctx, client, rootClient, "gone")
	defer gone.Release()
	if msg, err := echo.NewSRPCEchoerClient(hostClient).Echo(ctx, &echo.EchoMsg{Body: "host"}); err != nil || msg.GetBody() != "host" {
		t.Fatalf("host echo: %v %v", msg, err)
	}

	// Restart the upstream that holds them.
	second.kill()
	third := newBackend(3)
	third.refuse = "gone"
	current.Store(third)

	// The children answer from the new upstream under the IDs the client
	// holds, and the refused one is released.
	if msg, err := echo.NewSRPCEchoerClient(hostClient).Echo(ctx, &echo.EchoMsg{Body: "host"}); err != nil || msg.GetBody() != "host" {
		t.Fatalf("replayed host echo: %v %v", msg, err)
	}
	select {
	case <-gone.Done():
	case <-ctx.Done():
		t.Fatal("refused child was not released")
	}
	want := []string{"EchoClientStream", "Open device", "Open host", "Open gone"}
	if calls := third.callList(); len(calls) < len(want) || !slices.Equal(calls[:len(want)], want) {
		t.Fatalf("third upstream calls: %v", calls)
	}

	// Confirm reopened children retain their adoption before upstream release.
	third.waitAdopted(t, ctx, "device", "host")

	// The upstream's release of a child reaches the client.
	third.release("host")
	select {
	case <-host.Done():
	case <-ctx.Done():
		t.Fatal("released host still held")
	}
	if msg, err := echo.NewSRPCEchoerClient(deviceClient).Echo(ctx, &echo.EchoMsg{Body: "device"}); err != nil || msg.GetBody() != "device" {
		t.Fatalf("device echo: %v %v", msg, err)
	}
	select {
	case <-client.Done():
		t.Fatal("generation ended across a restart")
	default:
	}
}

// openChild opens the child named name beneath the resource on serves and
// returns the client's reference to it.
func openChild(t *testing.T, ctx context.Context, client *resource_client.Client, on srpc.Client, name string) resource_client.ResourceRef {
	t.Helper()
	var opened resource.ResourceReleasedResponse
	if err := on.ExecCall(ctx, childService, "Open", &echo.EchoMsg{Body: name}, &opened); err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	return client.CreateResourceReference(opened.GetResourceId())
}

// backend is one upstream resource server whose connections can be cut.
type backend struct {
	// adopted lists the names of adopted children.
	adopted []string
	// names indexes children by upstream resource ID.
	names map[uint32]string
	// gen numbers the backend.
	gen int
	// server serves the backend's resource service.
	server *srpc.Server
	// dead is set once the backend is killed.
	dead atomic.Bool
	// refuse names the child the backend refuses to open.
	refuse string

	// mtx guards the fields below.
	mtx sync.Mutex
	// calls lists the echo methods called, in order.
	calls []string
	// called wakes waitCalls.
	called chan struct{}
	// conns holds the server side of every connection.
	conns []net.Conn
	// releases releases each opened child by name.
	releases map[string]func()
}

// newBackend starts backend gen.
func newBackend(gen int) *backend {
	b := &backend{gen: gen, called: make(chan struct{}), releases: make(map[string]func()), names: make(map[uint32]string)}
	serverMux := srpc.NewMux()
	server := resource_server.NewResourceServer(b.mux())
	_ = resource.SRPCRegisterResourceService(serverMux, &adoptRecorder{SRPCResourceServiceServer: server, b: b})
	b.server = srpc.NewServer(serverMux)
	return b
}

// mux returns a resource serving the echo methods and opening children.
func (b *backend) mux() srpc.Mux {
	mux := srpc.NewMux(srpc.InvokerFunc(b.invoke))
	_ = echo.SRPCRegisterEchoer(mux, &echoer{b: b})
	return mux
}

// invoke serves the services the backend serves without generated code.
func (b *backend) invoke(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	switch serviceID {
	case childService:
		return true, b.openChild(strm)
	case viewService:
		return true, b.watchView(strm)
	}
	return false, nil
}

// openChild opens the child the request names beneath the resource the call
// runs on, answering with its ID in field 1 as an opener does.
func (b *backend) openChild(strm srpc.Stream) error {
	var req echo.EchoMsg
	if err := strm.MsgRecv(&req); err != nil {
		return err
	}
	b.call("Open " + req.GetBody())
	if req.GetBody() == b.refuse {
		return errors.New("refused")
	}
	owner, err := resource_server.MustGetResourceClientContext(strm.Context())
	if err != nil {
		return err
	}
	_, id, err := resource_server.ConstructChildResource(strm.Context(), func(context.Context) (srpc.Invoker, struct{}, func(), error) {
		return b.mux(), struct{}{}, nil, nil
	})
	if err != nil {
		return err
	}
	b.mtx.Lock()
	b.names[id] = req.GetBody()
	b.releases[req.GetBody()] = func() { owner.ReleaseResource(id) }
	b.mtx.Unlock()
	return strm.MsgSend(&resource.ResourceReleasedResponse{ResourceId: id})
}

// adoptRecorder serves the resource service and records each Adopt of a child
// the backend opened.
type adoptRecorder struct {
	resource.SRPCResourceServiceServer
	b *backend
}

// ResourceClient serves a client generation, recording its Adopt controls.
func (r *adoptRecorder) ResourceClient(strm resource.SRPCResourceService_ResourceClientStream) error {
	return r.SRPCResourceServiceServer.ResourceClient(&adoptStream{SRPCResourceService_ResourceClientStream: strm, b: r.b})
}

// adoptStream is a ResourceClient stream that records Adopt controls.
type adoptStream struct {
	resource.SRPCResourceService_ResourceClientStream
	b *backend
}

// Recv returns the next control, recording an Adopt of a child.
func (s *adoptStream) Recv() (*resource.ResourceClientRequest, error) {
	req, err := s.SRPCResourceService_ResourceClientStream.Recv()
	if adopt := req.GetAdopt(); adopt != nil {
		s.b.adopt(adopt.GetResourceId())
	}
	return req, err
}

// adopt records the Adopt of the child with resource ID id.
func (b *backend) adopt(id uint32) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if name, ok := b.names[id]; ok {
		b.adopted = append(b.adopted, name)
		close(b.called)
		b.called = make(chan struct{})
	}
}

// release releases the child opened as name.
func (b *backend) release(name string) {
	b.mtx.Lock()
	release := b.releases[name]
	b.mtx.Unlock()
	release()
}

// open opens one connection to the backend.
func (b *backend) open(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.dead.Load() {
		return nil, errors.New("upstream down")
	}
	serverConn, clientConn := net.Pipe()
	b.conns = append(b.conns, serverConn)
	go b.server.HandleStream(ctx, serverConn)
	prw := srpc.NewPacketReadWriter(clientConn)
	go prw.ReadPump(msgHandler, closeHandler)
	return prw, nil
}

// kill cuts every connection and refuses new ones.
func (b *backend) kill() {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.dead.Store(true)
	for _, conn := range b.conns {
		_ = conn.Close()
	}
}

// call records a call to method.
func (b *backend) call(method string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.calls = append(b.calls, method)
	close(b.called)
	b.called = make(chan struct{})
}

// callList returns the methods called so far.
func (b *backend) callList() []string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return append([]string(nil), b.calls...)
}

// waitCalls waits until each of methods was called.
func (b *backend) waitCalls(t *testing.T, ctx context.Context, methods ...string) {
	t.Helper()
	for {
		b.mtx.Lock()
		seen := make(map[string]bool)
		for _, m := range b.calls {
			seen[m] = true
		}
		wait := b.called
		b.mtx.Unlock()
		all := true
		for _, m := range methods {
			all = all && seen[m]
		}
		if all {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for calls %v", methods)
		case <-wait:
		}
	}
}

// echoer serves the echo methods for one backend. The first backend holds
// the calls the test leaves in flight across its restart.
type echoer struct {
	*echo.EchoServer
	b *backend
}

// Echo answers, or holds "hold" on the first backend.
func (e *echoer) Echo(ctx context.Context, msg *echo.EchoMsg) (*echo.EchoMsg, error) {
	e.b.call("Echo")
	if e.b.gen == 1 && msg.GetBody() == "hold" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return msg.CloneVT(), nil
}

// EchoServerStream sends the request as its state, then a change on every
// backend after the first, then waits.
func (e *echoer) EchoServerStream(msg *echo.EchoMsg, strm echo.SRPCEchoer_EchoServerStreamStream) error {
	e.b.call("EchoServerStream")
	if err := strm.Send(msg); err != nil {
		return err
	}
	if e.b.gen > 1 {
		if err := strm.Send(&echo.EchoMsg{Body: "changed"}); err != nil {
			return err
		}
	}
	<-strm.Context().Done()
	return nil
}

// EchoClientStream answers its first message.
func (e *echoer) EchoClientStream(strm echo.SRPCEchoer_EchoClientStreamStream) (*echo.EchoMsg, error) {
	e.b.call("EchoClientStream")
	return strm.Recv()
}

// DoNothing holds on the first backend.
func (e *echoer) DoNothing(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	e.b.call("DoNothing")
	if e.b.gen == 1 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &emptypb.Empty{}, nil
}

// waitAdopted waits until each child in names was adopted.
func (b *backend) waitAdopted(t *testing.T, ctx context.Context, names ...string) {
	t.Helper()
	b.waitRecorded(t, ctx, func() []string { return b.adopted }, names)
}

// waitRecorded waits until the records list returns, read under mtx, hold
// every entry of want.
func (b *backend) waitRecorded(t *testing.T, ctx context.Context, list func() []string, want []string) {
	t.Helper()
	for {
		// Read the records and the channel the next one closes.
		b.mtx.Lock()
		have := list()
		wait := b.called
		b.mtx.Unlock()

		// Stop once every wanted record is present.
		if !slices.ContainsFunc(want, func(w string) bool { return !slices.Contains(have, w) }) {
			return
		}

		// Wait for the next record.
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %v, have %v", want, have)
		case <-wait:
		}
	}
}

// EchoBidiStream holds an unclassified stream until the connection ends.
func (e *echoer) EchoBidiStream(strm echo.SRPCEchoer_EchoBidiStreamStream) error {
	e.b.call("EchoBidiStream")
	<-strm.Context().Done()
	return nil
}
