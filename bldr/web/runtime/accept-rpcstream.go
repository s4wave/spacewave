package web_runtime

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/bldr/util/framedstream"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// AcceptServiceWorkerRpcStreams accepts streams from a muxed connection and handles
// RpcStream protocol, routing them to the ServiceWorkerHost mux.
// This is used for the saucer fetch connection where C++ FetchClient sends
// RpcStream-protocol requests directly to ServiceWorkerHost/Fetch.
func (r *Remote) AcceptServiceWorkerRpcStreams(ctx context.Context, mc srpc.MuxedConn) error {
	for {
		muxedStream, err := mc.AcceptStream()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}

		go r.handleServiceWorkerRpcStream(ctx, muxedStream)
	}
}

// handleServiceWorkerRpcStream handles a single RpcStream for ServiceWorkerHost.
func (r *Remote) handleServiceWorkerRpcStream(ctx context.Context, rwc io.ReadWriteCloser) {
	defer rwc.Close()

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	stream := framedstream.New(subCtx, rwc)
	_ = rpcstream.HandleRpcStream(stream, r.GetServiceWorkerHost)
}
