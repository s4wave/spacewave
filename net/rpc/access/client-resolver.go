package bifrost_rpc_access

import (
	"context"
	"sync"

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
func (r *LookupRpcServiceResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Build the service lookup request and clear published invokers on exit.
	req := RequestFromDirective(r.dir)
	defer handler.ClearValues()

	// Resolve service availability across successive client lifetimes.
	var mtx sync.Mutex
	var clientCtx context.Context
	var clientCtxCancel context.CancelFunc
	var nonce uint64
ClientLoop:
	for {
		// Cancel the current client when the lookup directive ends.
		if ctx.Err() != nil {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			return context.Canceled
		}

		// Invalidate the current client context when its reference is released.
		var currNonce uint64
		clientReleased := func() {
			// Advance the client generation while canceling its lookup stream.
			mtx.Lock()
			if clientCtxCancel != nil {
				clientCtxCancel()
				clientCtxCancel = nil
			}
			nonce++
			currNonce = nonce
			mtx.Unlock()
		}

		// Replace published invokers with a lookup through the next client.
		handler.ClearValues()
		nextClient, relNextClient, err := r.svc(ctx, clientReleased)
		if err != nil {
			return err
		}

		// Bind the lookup context only if the next client is still retained.
		mtx.Lock()
		nextClientOk := currNonce == nonce
		if nextClientOk {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			clientCtx, clientCtxCancel = context.WithCancel(ctx)
		}
		mtx.Unlock()

		// Release a client invalidated before its lookup context was bound.
		if !nextClientOk {
			// client was released already
			if relNextClient != nil {
				relNextClient()
			}
			continue
		}

		// Open the retained client lookup stream and release it on failure.
		strm, err := nextClient.LookupRpcService(clientCtx, req)
		if err != nil {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			relNextClient()
			clientReleased()
			return err
		}

		// Publish service invokers until the client lookup stream ends.
		var valID uint32
		for {
			// Reacquire a client when the current lookup stream ends.
			resp, err := strm.Recv()
			if err != nil {
				relNextClient()
				clientReleased()
				continue ClientLoop
			}

			// Remove the published invoker when the remote service disappears.
			if removed := resp.GetRemoved(); removed && valID != 0 {
				_, _ = handler.RemoveValue(valID)
				valID = 0
			}

			// Publish a proxy invoker when the remote service becomes available.
			if exists := resp.GetExists(); exists && valID == 0 {
				var val bifrost_rpc.LookupRpcServiceValue = NewProxyInvoker(nextClient, req, r.waitAck)
				valID, _ = handler.AddValue(val)
			}

			// Mark the directive idle when the remote lookup has no pending work.
			if resp.GetIdle() {
				handler.MarkIdle(true)
			}
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = (*LookupRpcServiceResolver)(nil)
