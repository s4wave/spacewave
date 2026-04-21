package bifrost_rpc_access

import (
	context "context"
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
	req := RequestFromDirective(r.dir)
	defer handler.ClearValues()

	var mtx sync.Mutex
	var clientCtx context.Context
	var clientCtxCancel context.CancelFunc
	var nonce uint64
ClientLoop:
	for {
		if ctx.Err() != nil {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			return context.Canceled
		}

		var currNonce uint64
		clientReleased := func() {
			mtx.Lock()
			if clientCtxCancel != nil {
				clientCtxCancel()
				clientCtxCancel = nil
			}
			nonce++
			currNonce = nonce
			mtx.Unlock()
		}

		handler.ClearValues()
		nextClient, relNextClient, err := r.svc(ctx, clientReleased)
		if err != nil {
			return err
		}

		mtx.Lock()
		nextClientOk := currNonce == nonce
		if nextClientOk {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			clientCtx, clientCtxCancel = context.WithCancel(ctx)
		}
		mtx.Unlock()
		if !nextClientOk {
			// client was released already
			if relNextClient != nil {
				relNextClient()
			}
			continue
		}

		strm, err := nextClient.LookupRpcService(clientCtx, req)
		if err != nil {
			if clientCtxCancel != nil {
				clientCtxCancel()
			}
			relNextClient()
			clientReleased()
			return err
		}

		var valID uint32
		for {
			resp, err := strm.Recv()
			if err != nil {
				relNextClient()
				clientReleased()
				continue ClientLoop
			}

			if removed := resp.GetRemoved(); removed && valID != 0 {
				_, _ = handler.RemoveValue(valID)
				valID = 0
			}
			if exists := resp.GetExists(); exists && valID == 0 {
				var val bifrost_rpc.LookupRpcServiceValue = NewProxyInvoker(nextClient, req, r.waitAck)
				valID, _ = handler.AddValue(val)
			}
			if resp.GetIdle() {
				handler.MarkIdle(true)
			}
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*LookupRpcServiceResolver)(nil))
