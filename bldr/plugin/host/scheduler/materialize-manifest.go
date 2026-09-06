package plugin_host_scheduler

import (
	"context"
	"io"
	"regexp"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_manifest_materializer "github.com/s4wave/spacewave/bldr/manifest/materializer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/net/util/randstring"
)

// materializeManifest copies the selected manifest from src to dest through
// the materializer plugin's typed streaming service.
//
// The caller retains both cursors for the whole call. The scheduler serves the
// source encoded blocks to the plugin over a request-scoped BlockStore RPC
// service registered on the scheduler bus under a random service ID, restricted
// to RPCs originating from the named plugin. The request carries the resolved
// source transform configuration inline and the destination transform
// configuration; the destination volume ID selects the existing plugin-host
// ProxyVolume mounted inside the plugin.
//
// The response stream carries coalesced logical statistics and exactly one
// terminal successful message with the copied root. The helper requires that
// terminal root and a clean stream EOF before returning success, and returns
// the last observed statistics on every path. There is no fallback to native
// copying on RPC failure: the caller's routine owns retry. The helper performs
// no World mutation.
func (c *Controller) materializeManifest(
	ctx context.Context,
	pluginID string,
	dest, src *bucket_lookup.Cursor,
	concurrency int,
) (*bucket.ObjectRef, bucket_lookup.ObjectCopyStats, error) {
	var stats bucket_lookup.ObjectCopyStats

	// Run the whole operation on a child context so the request-scoped source
	// service and the resolved plugin client release together on any exit.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Serve the source encoded blocks over a request-scoped BlockStore RPC
	// service, reachable only by this plugin for the duration of the copy.
	sourceID := randstring.RandomIdentifier(16)
	sourceMux := srpc.NewMux()
	if err := sourceMux.Register(block_rpc.NewSRPCBlockStoreHandler(
		block_rpc_server.NewBlockStore(src.GetBucket()),
		sourceID,
	)); err != nil {
		return nil, stats, errors.Wrap(err, "register source block store rpc handler")
	}
	// Accept the plugin's direct server identity and, when the plugin runs in
	// a browser worker, the exact web-worker identity carrying the scheduler's
	// instance key. Both alternatives are anchored and fully escaped.
	serverIDRe := regexp.MustCompile(
		"^(" +
			regexp.QuoteMeta(bldr_plugin.PluginServerID(pluginID, "")) +
			"|" +
			regexp.QuoteMeta("web-worker/"+bldr_plugin.PluginServerID(pluginID, c.conf.GetInstanceKey())) +
			")$",
	)
	sourceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			ControllerID+"/materialize-manifest/"+sourceID,
			Version,
			"request-scoped source block store for the materializer plugin",
		),
		func(_ context.Context, _ func()) (srpc.Invoker, func(), error) {
			return sourceMux, nil, nil
		},
		nil,
		false,
		nil,
		[]string{sourceID},
		serverIDRe,
	)
	relSourceCtrl, err := c.bus.AddController(ctx, sourceCtrl, nil)
	if err != nil {
		return nil, stats, errors.Wrap(err, "add source block store rpc controller")
	}
	defer relSourceCtrl()

	// Build the typed request. The source ref carries its resolved inline
	// transform configuration instead of a transform conf ref, and the
	// destination keeps its bucket while selecting the plugin-host volume.
	sourceRef := src.GetRefWithOpArgs()
	sourceRef.TransformConfRef = nil
	sourceRef.TransformConf = src.GetTransformConf().CloneVT()

	destOpArgs := dest.GetOpArgs()
	destOpArgs.VolumeId = bldr_plugin.PluginVolumeID

	materializerServiceID := bldr_plugin.PluginServiceID(pluginID, bldr_manifest_materializer.SRPCMaterializerServiceID)
	req := &bldr_manifest_materializer.MaterializeManifestRequest{
		Source:                   sourceRef,
		Destination:              destOpArgs,
		DestinationTransformConf: dest.GetTransformConf().CloneVT(),
		Concurrency:              uint32(concurrency),
		SourceServiceId:          bldr_plugin.HostServiceIDPrefix + sourceID,
	}

	// Resolve the materializer plugin's RPC client. The lookup demands the
	// plugin through the existing scheduler WaitPluginClient path; cancel the
	// operation if the resolved client becomes invalid.
	clientSet, _, clientsRef, err := bifrost_rpc.ExLookupRpcClientSet(ctx, c.bus, materializerServiceID, "", true, cancel)
	if err != nil {
		return nil, stats, errors.Wrap(err, "resolve materializer rpc client")
	}
	defer clientsRef.Release()
	client := bldr_manifest_materializer.NewSRPCMaterializerClientWithServiceID(clientSet, materializerServiceID)

	// Send the typed request and receive coalesced progress responses until
	// the stream ends, retaining the latest statistics and terminal root.
	strm, err := client.MaterializeManifest(ctx, req)
	if err != nil {
		return nil, stats, errors.Wrap(err, "open materialize manifest stream")
	}
	defer strm.Close()

	var copiedRef *bucket.ObjectRef
	for {
		resp, err := strm.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, stats, errors.Wrap(err, "receive materialize manifest response")
			}
			break
		}
		if wireStats := resp.GetStats(); wireStats != nil {
			stats = bucket_lookup.ObjectCopyStats{
				BlocksSeen:         wireStats.GetBlocksSeen(),
				BlocksCopied:       wireStats.GetBlocksCopied(),
				BlocksExisting:     wireStats.GetBlocksExisting(),
				BlocksWritten:      wireStats.GetBlocksWritten(),
				BlocksDeduped:      wireStats.GetBlocksDeduped(),
				SubtreesSkipped:    wireStats.GetSubtreesSkipped(),
				LogicalSourceBytes: wireStats.GetLogicalSourceBytes(),
			}
		}
		if resp.GetCopiedRef() != nil {
			copiedRef = resp.GetCopiedRef()
		}
	}

	// Accept only a clean end of stream with a non-empty terminal root.
	if err := ctx.Err(); err != nil {
		return nil, stats, err
	}
	if copiedRef == nil {
		return nil, stats, errors.New("materializer stream ended without a copied root")
	}
	if copiedRef.GetRootRef().GetEmpty() {
		return nil, stats, errors.New("materializer copied root has an empty root block ref")
	}
	return copiedRef, stats, nil
}
