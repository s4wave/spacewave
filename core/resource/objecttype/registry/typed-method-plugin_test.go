package resource_objecttype_registry

import (
	"context"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// typedMethodPlugin records accepted methods independently of plugin construction.
type typedMethodPlugin struct {
	// afterAccept exposes the successor after the child accepts its operation.
	afterAccept func()
	// invokeErr is the accepted operation's returned failure, nil for success.
	invokeErr error

	// mtx guards the factory, invocation and ResourceClient session counts.
	mtx sync.Mutex
	// factories counts handler-created typed children under mtx.
	factories int
	// invocations counts accepted child operations under mtx.
	invocations int
	// sessions counts ResourceClient generations opened under mtx.
	sessions int
}

// client exposes the handler through a real ResourceServer and records generations.
func (h *typedMethodPlugin) client(t *testing.T) (srpc.Client, *resource_server.ResourceServer) {
	// Register the plugin's typed handler and its Resource service.
	t.Helper()
	root := srpc.NewMux()
	if err := sdk_registry.SRPCRegisterObjectTypeHandlerService(root, h); err != nil {
		t.Fatal(err)
	}
	server := resource_server.NewResourceServer(root)
	mux := srpc.NewMux()
	if err := server.Register(mux); err != nil {
		t.Fatal(err)
	}

	// Count real ResourceClient requests before the service starts each generation.
	invoker := srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		if serviceID == resource.SRPCResourceServiceServiceID && methodID == "ResourceClient" {
			h.mtx.Lock()
			h.sessions++
			h.mtx.Unlock()
		}
		return mux.InvokeMethod(serviceID, methodID, stream)
	})
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))), server
}

// InvokeObjectType creates one typed child within the real plugin Resource generation.
func (h *typedMethodPlugin) InvokeObjectType(ctx context.Context, req *sdk_registry.InvokeObjectTypeRequest) (*sdk_registry.InvokeObjectTypeResponse, error) {
	// Require the admitted object and its borrowed World engine.
	if req.GetTypeId() != "test/type" || req.GetObjectKey() != "test/object" || req.GetAttachedEngineResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Count each factory acquisition and return its real child Resource.
	h.mtx.Lock()
	h.factories++
	h.mtx.Unlock()
	id, err := owner.AddResource(srpc.InvokerFunc(h.invokeChild), nil)
	if err != nil {
		return nil, err
	}
	return &sdk_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

// invokeChild records acceptance before exposing a successor and returning failure.
func (h *typedMethodPlugin) invokeChild(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	// Accept exactly the explicit Ping operation after reading its request.
	if serviceID != "test.Child" || methodID != "Ping" {
		return false, nil
	}
	if err := stream.MsgRecv(&testPingMessage{}); err != nil {
		return true, err
	}
	h.mtx.Lock()
	h.invocations++
	h.mtx.Unlock()

	// Expose the replacement only after the operation has already been accepted.
	if h.afterAccept != nil {
		h.afterAccept()
	}
	if h.invokeErr != nil {
		return true, h.invokeErr
	}
	return true, stream.MsgSend(&testPingMessage{})
}

// counts returns a consistent snapshot of factory, accepted-call and session counts.
func (h *typedMethodPlugin) counts() (int, int, int) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.factories, h.invocations, h.sessions
}

// _ is a type assertion.
var _ sdk_registry.SRPCObjectTypeHandlerServiceServer = (*typedMethodPlugin)(nil)
