package bifrost_rpc_access

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// LookupRpcServiceResolver resolves a LookupRpcService directive with a RPC service.
type LookupRpcServiceResolver struct {
	dir     bifrost_rpc.LookupRpcService
	svc     AccessClientFunc
	waitAck bool
}

// NewLookupRpcServiceResolver constructs the directive resolver.
//
// if waitAck is set, waits for ack from the remote before starting the proxied rpc.
// note: usually you do not need waitAck set to true.
func NewLookupRpcServiceResolver(
	dir bifrost_rpc.LookupRpcService,
	svc AccessClientFunc,
	waitAck bool,
) *LookupRpcServiceResolver {
	return &LookupRpcServiceResolver{dir: dir, svc: svc, waitAck: waitAck}
}

// Resolve resolves the values, emitting them to the handler.
//
// Each client gets its own lookup context. The client released callback
// cancels that context, which ends the lookup stream and acquires the next
// client. A late callback from an earlier client cancels only its own context.
func (r *LookupRpcServiceResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Build the service lookup request and clear published invokers on exit.
	req := RequestFromDirective(r.dir)
	defer handler.ClearValues()

	// Look up the service through each successive client.
	for {
		handler.ClearValues()
		clientCtx, clientCtxCancel := context.WithCancel(ctx)
		err := r.resolveClient(clientCtx, clientCtxCancel, handler, req)
		clientCtxCancel()
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err != nil {
			return err
		}
	}
}

// resolveClient publishes service invokers through one client until the
// lookup stream ends or the client is released.
//
// Returns nil to acquire the next client, or an error to retry the resolver.
func (r *LookupRpcServiceResolver) resolveClient(
	ctx context.Context,
	released func(),
	handler directive.ResolverHandler,
	req *LookupRpcServiceRequest,
) error {
	// Acquire the client and hold its reference for the stream lifetime.
	client, relClient, err := r.svc(ctx, released)
	if err != nil {
		return err
	}
	if relClient != nil {
		defer relClient()
	}

	// Open the lookup stream. A released client acquires the next one.
	strm, err := client.LookupRpcService(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	// Publish service invokers until the lookup stream ends.
	var valID uint32
	for {
		resp, err := strm.Recv()
		if err != nil {
			return nil
		}

		// Remove the published invoker when the remote service disappears.
		if removed := resp.GetRemoved(); removed && valID != 0 {
			_, _ = handler.RemoveValue(valID)
			valID = 0
		}

		// Publish a proxy invoker when the remote service becomes available.
		if exists := resp.GetExists(); exists && valID == 0 {
			var val bifrost_rpc.LookupRpcServiceValue = NewProxyInvoker(client, req, r.waitAck)
			valID, _ = handler.AddValue(val)
		}

		// Mark the directive idle when the remote lookup has no pending work.
		if resp.GetIdle() {
			handler.MarkIdle(true)
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = (*LookupRpcServiceResolver)(nil)
