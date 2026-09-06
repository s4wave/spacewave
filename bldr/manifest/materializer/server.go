package bldr_manifest_materializer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// progressCoalesceBlocks is the minimum number of additional processed blocks
// between streamed progress responses.
const progressCoalesceBlocks int64 = 64

// Materializer implements the Materializer service: it copies a manifest
// object graph into a destination bucket with the existing copy engine.
type Materializer struct {
	// le is the logger.
	le *logrus.Entry
	// b is the controller bus used to resolve bucket handles.
	b bus.Bus
	// sfs is the transform step factory set.
	sfs *block_transform.StepFactorySet
}

// NewMaterializer constructs a new Materializer service.
func NewMaterializer(le *logrus.Entry, b bus.Bus, sfs *block_transform.StepFactorySet) *Materializer {
	return &Materializer{
		le:  le,
		b:   b,
		sfs: sfs,
	}
}

// MaterializeManifest copies the manifest object graph referenced by the
// request source into the request destination bucket, streaming coalesced
// progress snapshots and one terminal response carrying the copied root.
//
// The source is followed read-only and the destination is opened for writes
// through the bus; the service performs no World mutation, so publication and
// durability remain the caller's responsibility.
//
// When SourceServiceId is set, the source blocks are read through the
// BlockStore RPC service with that ID instead of the host-local source
// lookup; the source ref carries its resolved inline transform configuration.
//
// The handler runs on a child of the RPC stream context: the child cancels
// when the stream ends or the resolved source service disappears, which ends
// the copy and releases the RPC client and cursors with it.
func (m *Materializer) MaterializeManifest(
	req *MaterializeManifestRequest,
	strm SRPCMaterializer_MaterializeManifestStream,
) error {
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()

	// Validate the request before acquiring any handles.
	if req.GetSource().GetRootRef().GetEmpty() {
		return errors.New("source root ref cannot be empty")
	}
	if req.GetDestination().GetBucketId() == "" {
		return errors.New("destination bucket id cannot be empty")
	}

	// Build the destination cursor from the request parameters.
	dest, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		m.b,
		m.le,
		m.sfs,
		req.GetDestination().GetBucketId(),
		req.GetDestination().GetVolumeId(),
		req.GetDestinationTransformConf(),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "build destination cursor")
	}
	defer dest.Release()

	// Follow the source reference read-only. If the request carries a source
	// service ID, the source blocks are read through that BlockStore RPC
	// service instead of the host-local source lookup. The client reference
	// and base cursor live until the end of the operation, so their releases
	// defer here alongside the followed source cursor.
	var src *bucket_lookup.Cursor
	if req.GetSourceServiceId() == "" {
		src, err = bldr_manifest_world.FollowObjectRefReadOnly(ctx, dest, req.GetSource())
		if err != nil {
			return errors.Wrap(err, "follow source object ref")
		}
	} else {
		if req.GetSource().GetBucketId() == "" {
			return errors.New("source bucket id cannot be empty when source service id is set")
		}
		if !req.GetSource().GetTransformConfRef().GetEmpty() {
			return errors.New("source.transform_conf_ref must be empty; resolve the source transform configuration first")
		}

		// Resolve the BlockStore RPC client for the request-scoped service ID.
		// The lookup is bound to the handler cancel func so the service is
		// released when the copy finishes or the stream ends. There is no
		// fallback to the host-local source lookup: if the service is
		// unavailable the copy fails.
		serviceID := req.GetSourceServiceId()
		clientSet, _, clientsRef, err := bifrost_rpc.ExLookupRpcClientSet(ctx, m.b, serviceID, "", true, cancel)
		if err != nil {
			return errors.Wrap(err, "resolve source block store rpc client")
		}
		defer clientsRef.Release()

		rawStore := block_rpc_client.NewBlockStore(
			block_rpc.NewSRPCBlockStoreClientWithServiceID(clientSet, serviceID),
			0,
			true,
		)

		// The raw base cursor preserves the source-encoded bytes as delivered
		// by the RPC store; the inline transform conf in the source ref is
		// applied by FollowRef.
		base := bucket_lookup.NewCursor(
			ctx,
			m.b,
			m.le,
			m.sfs,
			rawStore,
			nil,
			nil,
			&bucket.BucketOpArgs{BucketId: req.GetSource().GetBucketId()},
			nil,
		)
		defer base.Release()

		src, err = base.FollowRef(ctx, req.GetSource())
		if err != nil {
			return errors.Wrap(err, "follow source object ref over rpc")
		}
	}
	defer src.Release()

	// Stream coalesced progress, sending a response after every 64 additional
	// processed blocks, and stop the copy when the stream is canceled.
	var lastSent int64
	progressCb := func(stats bucket_lookup.ObjectCopyStats) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if stats.BlocksSeen-lastSent < progressCoalesceBlocks {
			return nil
		}
		lastSent = stats.BlocksSeen
		return strm.Send(newMaterializeManifestResponse(&stats, nil))
	}

	// Copy the object graph with the existing copy engine.
	copiedRef, stats, err := bucket_lookup.CopyObjectToBucketWithProgress(
		ctx,
		dest,
		src,
		bldr_manifest.NewManifestBlock,
		int(req.GetConcurrency()),
		false,
		nil,
		progressCb,
	)
	if err != nil {
		return errors.Wrap(err, "copy manifest object")
	}

	// Stop before the terminal response if the stream was canceled.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Send the terminal response with the final stats and copied root.
	return strm.Send(newMaterializeManifestResponse(&stats, copiedRef))
}

// newMaterializeManifestResponse builds a response from a stats snapshot and
// an optional copied root reference.
func newMaterializeManifestResponse(
	stats *bucket_lookup.ObjectCopyStats,
	copiedRef *bucket.ObjectRef,
) *MaterializeManifestResponse {
	resp := &MaterializeManifestResponse{}
	if stats != nil {
		resp.Stats = newCopyStats(stats)
	}
	resp.CopiedRef = copiedRef
	return resp
}

// newCopyStats converts logical copy accounting to the wire type.
func newCopyStats(stats *bucket_lookup.ObjectCopyStats) *CopyStats {
	return &CopyStats{
		BlocksSeen:         stats.BlocksSeen,
		BlocksCopied:       stats.BlocksCopied,
		BlocksExisting:     stats.BlocksExisting,
		BlocksWritten:      stats.BlocksWritten,
		BlocksDeduped:      stats.BlocksDeduped,
		SubtreesSkipped:    stats.SubtreesSkipped,
		LogicalSourceBytes: stats.LogicalSourceBytes,
	}
}

// _ is a type assertion
var (
	_ SRPCMaterializerServer = ((*Materializer)(nil))
)
