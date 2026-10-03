package web_runtime

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/util/framedstream"
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
	// Release the service worker connection when request handling ends.
	defer rwc.Close()

	// Scope the service worker request stream to this handler.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Route framed requests to the service worker host.
	stream := framedstream.New(subCtx, rwc)
	_ = rpcstream.HandleRpcStream(stream, r.GetServiceWorkerHost)
}
