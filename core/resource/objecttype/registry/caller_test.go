package resource_objecttype_registry

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/sobject"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// callerTestHandler checks identity at native dispatch and retained child RPCs.
type callerTestHandler struct{}

// InvokeObjectType publishes a child that requires the dispatch identity.
func (*callerTestHandler) InvokeObjectType(ctx context.Context, req *sdk_registry.InvokeObjectTypeRequest) (*sdk_registry.InvokeObjectTypeResponse, error) {
	// Require the registry's caller, including its accepted participant.
	caller := objecttype.CallerFromContext(ctx)
	if caller.PeerID != req.GetObjectKey() || caller.Participant.GetPeerId() != caller.PeerID {
		return nil, errors.Errorf("dispatch caller = %+v, want %s", caller, req.GetObjectKey())
	}
	resources, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Independently retained children inherit identity from publication custody.
	id, err := resources.AddResource(srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		// Serve only the identity-checking test RPC.
		if serviceID != "test.Caller" || methodID != "Ping" {
			return false, nil
		}
		if err := stream.MsgRecv(&testPingMessage{}); err != nil {
			return true, err
		}

		// Require the granting caller in both request and retained contexts.
		got := objecttype.CallerFromContext(stream.Context())
		children, err := resource_server.MustGetResourceClientContext(stream.Context())
		if err != nil {
			return true, err
		}
		retained := objecttype.CallerFromContext(children.Context())
		if got.PeerID != caller.PeerID || !got.Participant.EqualVT(caller.Participant) || retained.PeerID != caller.PeerID {
			return true, errors.Errorf("child caller = %+v, retained = %+v, want %+v", got, retained, caller)
		}
		return true, stream.MsgSend(&testPingMessage{})
	}), nil)
	if err != nil {
		return nil, err
	}
	return &sdk_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

// TestNativeBridgeRetainsEachCaller checks two concurrent grants on the same
// native handler without relying on request context propagation across SRPC.
func TestNativeBridgeRetainsEachCaller(t *testing.T) {
	// Publish a native typed handler through the actual registry bridge.
	ctx := t.Context()
	testbed := world_testbed.MustDefault(t, ctx)
	registry, resources, client := newRegistryResourceClient(t, ctx)
	t.Cleanup(resources.Release)

	// Serve the caller-restoring handler through the Resource protocol.
	handler := sdk_registry.NewCallerHandler(&callerTestHandler{})
	handlerRoot := srpc.NewMux()
	if err := sdk_registry.SRPCRegisterObjectTypeHandlerService(handlerRoot, handler); err != nil {
		t.Fatal(err)
	}
	handlerService := srpc.NewMux()
	if err := resource_server.NewResourceServer(handlerRoot, "").Register(handlerService); err != nil {
		t.Fatal(err)
	}

	// Register the attached native handler with the real bridge.
	handlerID, err := resources.AttachResource(ctx, "caller-handler", handlerService)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RegisterObjectType(ctx, &sdk_registry.RegisterObjectTypeRequest{
		TypeId: "test/caller", PluginId: "caller-test", AttachedHandlerResourceId: handlerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := logrus.NewEntry(logrus.New())
	release, err := testbed.Bus.AddController(ctx, NewBridgeController(logger, testbed.Bus, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Resolve the bridged factory used by production typed dispatch.
	typed, ref, err := objecttype.ExLookupObjectType(ctx, testbed.Bus, "test/caller")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)

	// Keep both grants alive, then invoke each without any caller on the request.
	var children []srpc.Client
	for _, peerID := range []peer.ID{peer.ID("\x00\x01a"), peer.ID("\x00\x01b")} {
		caller := objecttype.Caller{
			PeerID: peerID.String(),
			Participant: &sobject.SOParticipantConfig{
				PeerId: peerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_WRITER,
			},
		}
		factoryCtx := objecttype.WithCaller(ctx, caller)
		invoker, cleanup, err := typed.GetFactory()(factoryCtx, logger, testbed.Bus, testbed.Engine, nil, caller.PeerID)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cleanup)
		children = append(children, srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))))
	}
	for _, child := range children {
		if err := child.ExecCall(ctx, "test.Caller", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
			t.Fatal(err)
		}
	}
}
