package bldr_manifest

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/bldr/util/valuelist"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// FetchManifestViaRpc resolves FetchManifest calling an RPC service.
func FetchManifestViaRpc(
	ctx context.Context,
	dir FetchManifest,
	// SRPCManifestFetchClient -> FetchManifest
	clientFn func(ctx context.Context, in *FetchManifestRequest) (SRPCManifestFetch_FetchManifestClient, error),
	hnd directive.ResolverHandler,
	returnOnIdle bool,
	le *logrus.Entry,
) error {
	strm, err := clientFn(ctx, NewFetchManifestRequest(dir))
	if err != nil {
		return err
	}
	defer strm.Close()

	return valuelist.WatchDirectiveViaStream[*FetchManifestValue, *FetchManifestResponse](
		ctx,
		strm,
		hnd,
		hnd.MarkIdle,
		returnOnIdle,
		le,
	)
}

// FetchManifestViaRpc resolves FetchManifest calling an RPC service by looking up a client set.
func FetchManifestViaRpcLookupClientSet(
	rctx context.Context,
	b bus.Bus,
	dir FetchManifest,
	serviceID string,
	clientID string,
	waitOneClient bool,
	hnd directive.ResolverHandler,
	returnOnIdle bool,
	le *logrus.Entry,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	clientSet, _, ref, err := bifrost_rpc.ExLookupRpcClientSet(
		ctx,
		b,
		serviceID,
		clientID,
		waitOneClient,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer ref.Release()

	srv := NewSRPCManifestFetchClientWithServiceID(clientSet, serviceID)
	return FetchManifestViaRpc(ctx, dir, srv.FetchManifest, hnd, returnOnIdle, le)
}
