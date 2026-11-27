package bldr_manifest_materializer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
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
// The source is followed read-only, and every destination block is written
// through the BlockStore RPC service DestinationServiceId selects, over storage
// the caller holds until the caller references the copied root. The service
// performs no World mutation, so publication and durability remain the caller's
// responsibility.
//
// When SourceServiceId is set, the source blocks are read through the
// BlockStore RPC service with that ID instead of the host-local source
// lookup. The source ref carries its resolved inline transform configuration.
//
// The handler runs on a child of the RPC stream context: the child cancels
// when the stream ends or a resolved block store service disappears.
// Cancellation ends the copy and releases the RPC client and cursors.
func (m *Materializer) MaterializeManifest(
	req *MaterializeManifestRequest,
	strm SRPCMaterializer_MaterializeManifestStream,
) error {
	// Run the handler on a child of the RPC stream context.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()

	// Validate the request before acquiring any handles.
	if req.GetSource().GetRootRef().GetEmpty() {
		return errors.New("source root ref cannot be empty")
	}
	if req.GetDestination().GetBucketId() == "" {
		return errors.New("destination bucket id cannot be empty")
	}
	if req.GetDestinationServiceId() == "" {
		return errors.New("destination service id cannot be empty")
	}

	// Write the destination through the caller's BlockStore RPC service, so
	// every block lands in storage the caller holds. There is no fallback to
	// a bucket handle of this process, whose writes nothing would hold.
	destStore, destRef, err := m.lookupBlockStore(ctx, req.GetDestinationServiceId(), false, cancel)
	if err != nil {
		return errors.Wrap(err, "resolve destination block store rpc client")
	}
	defer destRef.Release()
	destBase := bucket_lookup.NewCursor(ctx, m.b, m.le, m.sfs, destStore, nil, nil, req.GetDestination(), nil)
	defer destBase.Release()
	dest, err := destBase.FollowRef(ctx, &bucket.ObjectRef{
		BucketId:      req.GetDestination().GetBucketId(),
		TransformConf: req.GetDestinationTransformConf().CloneVT(),
	})
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
		// There is no fallback to the host-local source lookup: if the
		// service is unavailable the copy fails.
		rawStore, srcRef, err := m.lookupBlockStore(ctx, req.GetSourceServiceId(), true, cancel)
		if err != nil {
			return errors.Wrap(err, "resolve source block store rpc client")
		}
		defer srcRef.Release()

		// The raw base cursor preserves the source-encoded bytes as delivered
		// by the RPC store. The inline transform conf in the source ref is
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

// lookupBlockStore resolves the BlockStore RPC service with serviceID and
// returns a store over it, read-only when readOnly is set, with the reference
// that keeps the service. The lookup is bound to cancel, so the handler ends
// when the service disappears.
func (m *Materializer) lookupBlockStore(
	ctx context.Context,
	serviceID string,
	readOnly bool,
	cancel func(),
) (*block_rpc_client.BlockStore, directive.Reference, error) {
	clientSet, _, ref, err := bifrost_rpc.ExLookupRpcClientSet(ctx, m.b, serviceID, "", true, cancel)
	if err != nil {
		return nil, nil, err
	}
	client := block_rpc.NewSRPCBlockStoreClientWithServiceID(clientSet, serviceID)
	return block_rpc_client.NewBlockStore(client, 0, readOnly), ref, nil
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
	_ SRPCMaterializerServer = (*Materializer)(nil)
)
