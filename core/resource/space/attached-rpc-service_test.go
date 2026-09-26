package resource_space

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	net_testbed "github.com/s4wave/spacewave/net/testbed"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// attachedEchoServiceID is the echo service ID behind the "attached/" prefix.
const attachedEchoServiceID = "attached/" + echo.SRPCEchoerServiceID

// attachedRpcServiceStream supplies a caller context and records readiness.
type attachedRpcServiceStream struct {
	// Stream supplies default stream methods for the test double.
	srpc.Stream
	// ctx is returned to BindAttachedRpcService.
	ctx context.Context
	// ready records the first readiness response.
	ready chan *s4wave_space.BindAttachedRpcServiceResponse
	// checkReady verifies the route before recording readiness.
	checkReady func() error
}

// newAttachedRpcServiceStream creates a bind stream with one readiness slot.
func newAttachedRpcServiceStream(ctx context.Context) *attachedRpcServiceStream {
	return &attachedRpcServiceStream{
		ctx:   ctx,
		ready: make(chan *s4wave_space.BindAttachedRpcServiceResponse, 1),
	}
}

// Context returns the bind stream context.
func (s *attachedRpcServiceStream) Context() context.Context {
	return s.ctx
}

// Send records that the attached RPC service is callable.
func (s *attachedRpcServiceStream) Send(resp *s4wave_space.BindAttachedRpcServiceResponse) error {
	if s.checkReady != nil {
		if err := s.checkReady(); err != nil {
			return err
		}
	}
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
	tb, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true, NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	resource := newTestSpaceContentsResource(t, le, tb.Bus, nil, &plugin_space.Config{
		SpaceId:  "space-a",
		EngineId: "engine-a",
	})
	sibling := newTestSpaceContentsResource(t, le, tb.Bus, nil, &plugin_space.Config{
		SpaceId:  "space-b",
		EngineId: "engine-a",
	})
	gen := waitSpaceRuntimeGeneration(t, resource.runtime, nil)
	siblingGen := waitSpaceRuntimeGeneration(t, sibling.runtime, nil)

	attachedResources := newSpaceRecordingResourceClient(ctx)
	attachedID := addAttachedEchoResource(t, attachedResources)
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

	// The route resolves only on the Space runtime bus.
	if err := invokeAttachedEcho(ctx, gen.GetBus(), "attached"); err != nil {
		t.Fatal(err)
	}
	if countAttachedEcho(t, ctx, tb.Bus) != 0 {
		t.Fatal("daemon bus resolved attached service")
	}
	if countAttachedEcho(t, ctx, siblingGen.GetBus()) != 0 {
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
	if countAttachedEcho(t, ctx, gen.GetBus()) != 0 {
		t.Fatal("released attachment remained callable")
	}
}

func TestBindAttachedRpcServicePreEndedOwner(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := net_testbed.NewTestbed(ctx, le, net_testbed.TestbedOpts{NoEcho: true, NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	resource := newTestSpaceContentsResource(t, le, tb.Bus, nil, &plugin_space.Config{
		SpaceId:  "space-a",
		EngineId: "engine-a",
	})
	gen := waitSpaceRuntimeGeneration(t, resource.runtime, nil)

	ownerCtx, cancelOwner := context.WithCancel(ctx)
	attachedResources := newSpaceRecordingResourceClient(ownerCtx)
	attachedID := addAttachedEchoResource(t, attachedResources)
	cancelOwner()

	stream := newAttachedRpcServiceStream(resource_server.WithResourceClientContext(ctx, attachedResources))
	err = resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "attached/",
	}, stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BindAttachedRpcService error = %v, want %v", err, context.Canceled)
	}
	select {
	case <-stream.ready:
		t.Fatal("pre-ended owner received readiness")
	default:
	}
	if countAttachedEcho(t, ctx, gen.GetBus()) != 0 {
		t.Fatal("pre-ended owner retained attached service route")
	}
}

// addAttachedEchoResource adds an attached resource serving the echo service.
func addAttachedEchoResource(t *testing.T, resources *spaceRecordingResourceClient) uint32 {
	t.Helper()
	mux := srpc.NewMux()
	if err := mux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID)); err != nil {
		t.Fatal(err)
	}
	id, err := resources.AddResource(mux, nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// invokeAttachedEcho calls the attached echo route on b.
func invokeAttachedEcho(ctx context.Context, b bus.Bus, body string) error {
	invoker := bifrost_rpc.NewInvoker(b, "", false)
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	response, err := echo.NewSRPCEchoerClientWithServiceID(client, attachedEchoServiceID).
		Echo(ctx, &echo.EchoMsg{Body: body})
	if err != nil {
		return err
	}
	if response.GetBody() != body {
		return errors.Errorf("attached echo body = %q, want %q", response.GetBody(), body)
	}
	return nil
}

// countAttachedEcho returns the number of attached echo routes on b.
func countAttachedEcho(t *testing.T, ctx context.Context, b bus.Bus) int {
	t.Helper()
	values, _, valuesRef, err := bifrost_rpc.ExLookupRpcService(ctx, b, attachedEchoServiceID, "", false, nil)
	if valuesRef != nil {
		valuesRef.Release()
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(values)
}
