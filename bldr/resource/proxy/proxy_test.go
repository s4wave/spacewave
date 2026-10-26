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
	},
	childService: {
		"Open": resource_proxy.Open,
	},
}

// TestProxyCarriesCallsAcrossRestart runs a resource client through the proxy
// while its upstream restarts twice, and the generation lives on. Across the
// first restart a held Retry call is answered by the new upstream, a Once call
// in flight fails with a reset, a Reopen stream continues without repeating
// its state. Across the second, the children the client opened, one nested in
// another, open again and keep their IDs, while the one the new upstream
// refuses is released. An upstream release reaches the client by the proxy's
// ID.
func TestProxyCarriesCallsAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// Serve a client through the proxy to the first upstream.
	var current atomic.Pointer[backend]
	current.Store(newBackend(1))
	proxy := resource_proxy.NewProxy(ctx, rules, func(context.Context) (resource.SRPCResourceServiceClient, error) {
		b := current.Load()
		if b.dead.Load() {
			return nil, errors.New("upstream down")
		}
		return resource.NewSRPCResourceServiceClient(srpc.NewClient(b.open)), nil
	})
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
	first.waitCalls(t, ctx, "Echo", "DoNothing")

	// Restart the upstream.
	first.kill()
	second := newBackend(2)
	current.Store(second)

	// The Retry call is answered and the Once call is reset.
	if err := <-retried; err != nil {
		t.Fatalf("retried echo: %v", err)
	}
	if err := <-once; !errors.Is(err, srpc.ErrReset) {
		t.Fatalf("once call: got %v, want reset", err)
	}

	// The watch continues with the new upstream's change only.
	if msg, err := watch.Recv(); err != nil || msg.GetBody() != "changed" {
		t.Fatalf("reopened watch message: %v %v", msg, err)
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
	want := []string{"Open device", "Open host", "Open gone"}
	if calls := third.callList(); len(calls) < len(want) || !slices.Equal(calls[:len(want)], want) {
		t.Fatalf("third upstream calls: %v", calls)
	}

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
	b := &backend{gen: gen, called: make(chan struct{}), releases: make(map[string]func())}
	serverMux := srpc.NewMux()
	_ = resource_server.NewResourceServer(b.mux()).Register(serverMux)
	b.server = srpc.NewServer(serverMux)
	return b
}

// mux returns a resource serving the echo methods and opening children.
func (b *backend) mux() srpc.Mux {
	mux := srpc.NewMux(srpc.InvokerFunc(b.openChild))
	_ = echo.SRPCRegisterEchoer(mux, &echoer{b: b})
	return mux
}

// openChild opens the child the request names beneath the resource the call
// runs on, answering with its ID in field 1 as an opener does.
func (b *backend) openChild(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != childService {
		return false, nil
	}
	var req echo.EchoMsg
	if err := strm.MsgRecv(&req); err != nil {
		return true, err
	}
	b.call("Open " + req.GetBody())
	if req.GetBody() == b.refuse {
		return true, errors.New("refused")
	}
	owner, err := resource_server.MustGetResourceClientContext(strm.Context())
	if err != nil {
		return true, err
	}
	_, id, err := resource_server.ConstructChildResource(strm.Context(), func(context.Context) (srpc.Invoker, struct{}, func(), error) {
		return b.mux(), struct{}{}, nil, nil
	})
	if err != nil {
		return true, err
	}
	b.mtx.Lock()
	b.releases[req.GetBody()] = func() { owner.ReleaseResource(id) }
	b.mtx.Unlock()
	return true, strm.MsgSend(&resource.ResourceReleasedResponse{ResourceId: id})
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
