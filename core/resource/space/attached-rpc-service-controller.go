package resource_space

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// AttachedRpcServiceController routes the RPC services under a prefix to an
// attached client and signals when the route can resolve calls.
type AttachedRpcServiceController struct {
	// RpcServiceController provides the RPC route after it receives a context.
	*bifrost_rpc.RpcServiceController
	// ready closes after RpcServiceController.Execute sets its context.
	ready chan struct{}
	// readyClosed records that ready was closed; Execute may run more than once
	// if ControllerBus restarts the controller after an error.
	readyClosed atomic.Bool
}

// NewAttachedRpcServiceController constructs a route of the services under
// prefix to client. The prefix is stripped before the call reaches client.
func NewAttachedRpcServiceController(prefix string, client srpc.Invoker) *AttachedRpcServiceController {
	return &AttachedRpcServiceController{
		RpcServiceController: bifrost_rpc.NewRpcServiceController(
			controller.NewInfo(
				"core/resource/space/attached-rpc-service/"+prefix,
				controller.MustParseVersion("0.0.1"),
				"attached RPC service route",
			),
			bifrost_rpc.NewRpcServiceBuilder(client),
			[]string{prefix},
			true,
			nil,
			nil,
			nil,
		),
		ready: make(chan struct{}),
	}
}

// Ready returns a channel closed once the route can resolve calls.
func (c *AttachedRpcServiceController) Ready() <-chan struct{} {
	return c.ready
}

// Execute initializes the RPC service controller and then signals readiness.
func (c *AttachedRpcServiceController) Execute(ctx context.Context) error {
	err := c.RpcServiceController.Execute(ctx)
	if c.readyClosed.CompareAndSwap(false, true) {
		close(c.ready)
	}
	return err
}

// _ is a type assertion
var _ controller.Controller = (*AttachedRpcServiceController)(nil)
