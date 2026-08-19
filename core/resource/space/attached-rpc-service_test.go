package resource_space

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	net_testbed "github.com/s4wave/spacewave/net/testbed"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// attachedRpcServiceStream supplies a caller context and records readiness.
type attachedRpcServiceStream struct {
	// Stream supplies default stream methods for the test double.
	srpc.Stream
	// ctx is returned to BindAttachedRpcService.
	ctx context.Context
	// ready records the first readiness response.
	ready chan *s4wave_space.BindAttachedRpcServiceResponse
}

// newAttachedRpcServiceStream creates a bind stream with one readiness slot.
func newAttachedRpcServiceStream(ctx context.Context) *attachedRpcServiceStream {
	return &attachedRpcServiceStream{ctx: ctx, ready: make(chan *s4wave_space.BindAttachedRpcServiceResponse, 1)}
}

// Context returns the bind stream context.
func (s *attachedRpcServiceStream) Context() context.Context {
	return s.ctx
}

// Send records that the attached RPC service is callable.
func (s *attachedRpcServiceStream) Send(resp *s4wave_space.BindAttachedRpcServiceResponse) error {
	s.ready <- resp
	return nil
}

// SendAndClose records readiness before closing the response direction.
func (s *attachedRpcServiceStream) SendAndClose(resp *s4wave_space.BindAttachedRpcServiceResponse) error {
	return s.Send(resp)
}

// MsgRecv implements srpc.Stream.
func (*attachedRpcServiceStream) MsgRecv(srpc.Message) error {
	return nil
}

// MsgSend implements srpc.Stream.
func (*attachedRpcServiceStream) MsgSend(srpc.Message) error {
	return nil
}

// CloseSend implements srpc.Stream.
func (*attachedRpcServiceStream) CloseSend() error {
	return nil
}

// Close implements srpc.Stream.
func (*attachedRpcServiceStream) Close() error {
	return nil
}

func TestBindAttachedRpcService(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	attachedBus, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true, NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	siblingBus, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true, NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}

	resource := NewSpaceContentsResource(le, attachedBus.Bus, nil, "space-a", "engine-a")
	runtime := &spaceRuntime{bus: attachedBus.Bus, done: make(chan struct{})}
	resource.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		resource.runtime = runtime
	})

	attachedResources := newSpaceRecordingResourceClient(ctx)
	attachedMux := srpc.NewMux()
	if err := attachedMux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID)); err != nil {
		t.Fatal(err)
	}
	attachedID, err := attachedResources.AddResource(attachedMux, nil)
	if err != nil {
		t.Fatal(err)
	}

	bindCtx := resource_server.WithResourceClientContext(ctx, attachedResources)
	stream := newAttachedRpcServiceStream(bindCtx)
	bindDone := make(chan error, 1)
	go func() {
		bindDone <- resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
			AttachedResourceId: attachedID,
			ServiceIdPrefix:    "attached/",
		}, stream)
	}()
	<-stream.ready

	invoker := bifrost_rpc.NewInvoker(attachedBus.Bus, "", false)
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	response, err := echo.NewSRPCEchoerClientWithServiceID(client, "attached/"+echo.SRPCEchoerServiceID).Echo(ctx, &echo.EchoMsg{Body: "attached"})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetBody() != "attached" {
		t.Fatalf("echo body = %q, want attached", response.GetBody())
	}

	values, _, valuesRef, err := bifrost_rpc.ExLookupRpcService(ctx, siblingBus.Bus, "attached/"+echo.SRPCEchoerServiceID, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if valuesRef != nil {
		valuesRef.Release()
	}
	if len(values) != 0 {
		t.Fatal("sibling Space resolved attached service")
	}

	duplicate := newAttachedRpcServiceStream(bindCtx)
	if err := resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "attached/",
	}, duplicate); err == nil {
		t.Fatal("duplicate prefix binding succeeded")
	}
	if err := resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		ServiceIdPrefix: "other/",
	}, newAttachedRpcServiceStream(bindCtx)); err == nil {
		t.Fatal("zero attachment ID binding succeeded")
	}
	if err := resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "attached",
	}, newAttachedRpcServiceStream(bindCtx)); err == nil {
		t.Fatal("unterminated prefix binding succeeded")
	}
	if err := resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "/attached/",
	}, newAttachedRpcServiceStream(bindCtx)); err == nil {
		t.Fatal("root prefix binding succeeded")
	}
	if err := resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    strings.Repeat("a", 256) + "/",
	}, newAttachedRpcServiceStream(bindCtx)); err == nil {
		t.Fatal("oversized prefix binding succeeded")
	}

	if !attachedResources.ReleaseResource(attachedID) {
		t.Fatal("attached resource release failed")
	}
	if err := <-bindDone; err != nil {
		t.Fatalf("bind returned %v after attached resource release", err)
	}
	values, _, valuesRef, err = bifrost_rpc.ExLookupRpcService(ctx, attachedBus.Bus, "attached/"+echo.SRPCEchoerServiceID, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if valuesRef != nil {
		valuesRef.Release()
	}
	if len(values) != 0 {
		t.Fatal("released attachment remained callable")
	}
}

func TestBindAttachedRpcServicePreEndedOwner(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	testbed, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true, NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}

	resource := NewSpaceContentsResource(le, testbed.Bus, nil, "space-a", "engine-a")
	runtime := &spaceRuntime{bus: testbed.Bus, done: make(chan struct{})}
	resource.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		resource.runtime = runtime
	})

	ownerCtx, cancelOwner := context.WithCancel(ctx)
	attachedResources := newSpaceRecordingResourceClient(ownerCtx)
	attachedID, err := attachedResources.AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelOwner()

	stream := newAttachedRpcServiceStream(resource_server.WithResourceClientContext(ctx, attachedResources))
	err = resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "ended/",
	}, stream)
	if err != context.Canceled {
		t.Fatalf("BindAttachedRpcService error = %v, want %v", err, context.Canceled)
	}
	select {
	case <-stream.ready:
		t.Fatal("pre-ended owner received readiness")
	default:
	}

	values, _, valuesRef, err := bifrost_rpc.ExLookupRpcService(ctx, testbed.Bus, "ended/service", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if valuesRef != nil {
		valuesRef.Release()
	}
	if len(values) != 0 {
		t.Fatal("pre-ended owner retained attached service route")
	}
}
