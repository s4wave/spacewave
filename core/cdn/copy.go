package cdn

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// v86ImageEdgePreds are the five graph predicates that bind a V86Image to its
// UnixFS asset objects. Copy-from-CDN preserves each edge's target object key
// verbatim so the content-addressed blocks the destination Space fetches
// overlap with the CDN block store and dedup holds.
var v86ImageEdgePreds = []string{
	string(s4wave_vm.PredV86ImageWasm),
	string(s4wave_vm.PredV86ImageBiosSeabios),
	string(s4wave_vm.PredV86ImageBiosVgabios),
	string(s4wave_vm.PredV86ImageKernel),
	string(s4wave_vm.PredV86ImageRootfs),
}

const legacySpacewaveV86ImageTypeID = "spacewave/vm/image/v86"

// V86ImageCopyProgressFunc receives cumulative block-copy progress.
type V86ImageCopyProgressFunc func(bucket_lookup.ObjectCopyStats) error

// CopyV86ImageFromCdn copies a V86Image (metadata block plus the five asset
// edges) from the CDN WorldState into a user-owned destination WorldState.
// The caller is responsible for providing WorldState handles already scoped
// to their mount: source restriction is enforced by whatever mounted =src=
// (the read-only CDN Space), and write authorization is enforced by whatever
// mounted =dst= (session membership / RBAC on the user Space).
//
// Edge target object keys are preserved verbatim; UnixFS asset objects and
// their underlying blocks are content-addressed so the destination block
// store resolves them against the CDN block store without a re-upload.
//
// Fails loud when =dst= is read-only: the underlying ApplyWorldOp /
// SetGraphQuad calls propagate the read-only error from the engine.
func CopyV86ImageFromCdn(
	ctx context.Context,
	src world.WorldState,
	dst world.WorldState,
	srcObjectKey string,
	dstObjectKey string,
) error {
	return CopyV86ImageFromCdnWithProgress(ctx, src, dst, srcObjectKey, dstObjectKey, nil)
}

// CopyV86ImageFromCdnWithProgress copies a V86Image and reports cumulative
// block accounting as its asset objects move into the destination bucket.
func CopyV86ImageFromCdnWithProgress(
	ctx context.Context,
	src world.WorldState,
	dst world.WorldState,
	srcObjectKey string,
	dstObjectKey string,
	progress V86ImageCopyProgressFunc,
) error {
	// Validate source and destination object keys.
	if srcObjectKey == "" {
		return errors.New("source object key is required")
	}
	if dstObjectKey == "" {
		return errors.New("destination object key is required")
	}

	// Read the source image metadata.
	img, err := readCdnV86Image(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read v86 image %q from cdn", srcObjectKey)
	}

	// Read the source image's asset edges.
	edges, err := readV86ImageEdges(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read v86 image edges for %q", srcObjectKey)
	}

	// Check whether the destination image can be created.
	dstImageExists, err := checkDstV86Image(ctx, dst, dstObjectKey, img)
	if err != nil {
		return errors.Wrap(err, "destination write check")
	}

	// Create the destination image when it is absent.
	if !dstImageExists {
		createdAt := img.GetCreatedAt().AsTime()
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		op := s4wave_vm.NewCreateV86ImageOp(dstObjectKey, img, createdAt)
		if _, _, err := dst.ApplyWorldOp(ctx, op, ""); err != nil {
			return errors.Wrap(err, "apply create v86 image op on destination")
		}
	}

	// Copy each asset object and link its graph edge.
	var copiedStats bucket_lookup.ObjectCopyStats
	for pred, targetKey := range edges {
		if targetKey == "" {
			continue
		}
		baseStats := copiedStats
		stats, err := ensureCopiedWorldObject(ctx, src, dst, targetKey, func(current bucket_lookup.ObjectCopyStats) error {
			if progress == nil {
				return nil
			}
			return progress(addObjectCopyStats(baseStats, current))
		})
		if err != nil {
			return errors.Wrapf(err, "copy %s asset object %q to destination", pred, targetKey)
		}
		copiedStats = addObjectCopyStats(copiedStats, stats)
		quad := world.NewGraphQuadWithKeys(dstObjectKey, pred, targetKey, "")
		if err := dst.SetGraphQuad(ctx, quad); err != nil {
			return errors.Wrapf(err, "set %s edge on destination", pred)
		}
	}

	// Report completion after all asset edges are durable.
	return nil
}

// ensureCopiedWorldObject creates objectKey in dst from src when the destination
// does not already have it. V86Image edges point at UnixFS objects, and the
// world graph requires both endpoints to exist before SetGraphQuad can link
// them.
func ensureCopiedWorldObject(
	ctx context.Context,
	src, dst world.WorldState,
	objectKey string,
	progress bucket_lookup.ObjectCopyProgress,
) (bucket_lookup.ObjectCopyStats, error) {
	if objectKey == "" {
		return bucket_lookup.ObjectCopyStats{}, nil
	}
	_, found, err := dst.GetObject(ctx, objectKey)
	if err != nil {
		return bucket_lookup.ObjectCopyStats{}, errors.Wrap(err, "probe destination object")
	}
	if found {
		if err := ensureCopiedObjectType(ctx, src, dst, objectKey); err != nil {
			return bucket_lookup.ObjectCopyStats{}, err
		}
		return bucket_lookup.ObjectCopyStats{}, nil
	}

	srcObj, srcFound, err := src.GetObject(ctx, objectKey)
	if err != nil {
		return bucket_lookup.ObjectCopyStats{}, errors.Wrap(err, "get source object")
	}
	if !srcFound {
		return bucket_lookup.ObjectCopyStats{}, errors.Errorf("source object %q not found", objectKey)
	}
	rootCtor, err := lookupCopyRootCtor(ctx, src, objectKey)
	if err != nil {
		return bucket_lookup.ObjectCopyStats{}, err
	}
	srcRef, _, err := srcObj.GetRootRef(ctx)
	if err != nil {
		return bucket_lookup.ObjectCopyStats{}, errors.Wrap(err, "get source object root")
	}

	var dstRef *bucket.ObjectRef
	var stats bucket_lookup.ObjectCopyStats
	err = dst.AccessWorldState(ctx, nil, func(dstCursor *bucket_lookup.Cursor) error {
		return srcObj.AccessWorldState(ctx, srcRef, func(srcCursor *bucket_lookup.Cursor) error {
			var copyErr error
			dstRef, stats, copyErr = bucket_lookup.CopyObjectToBucketWithProgress(
				ctx,
				dstCursor,
				srcCursor,
				rootCtor,
				1,
				false,
				nil,
				progress,
			)
			return copyErr
		})
	})
	if err != nil {
		return stats, errors.Wrap(err, "copy object blocks")
	}
	if _, err := dst.CreateObject(ctx, objectKey, dstRef); err != nil {
		return stats, errors.Wrap(err, "create destination object")
	}
	if err := ensureCopiedObjectType(ctx, src, dst, objectKey); err != nil {
		return stats, err
	}
	return stats, nil
}

func lookupCopyRootCtor(ctx context.Context, src world.WorldState, objectKey string) (block.Ctor, error) {
	fsType, _, err := unixfs_world.LookupFsType(ctx, src, objectKey)
	if err == nil {
		ctor, _, err := unixfs_world.GetFSRootWithType(fsType)
		return ctor, err
	}

	typeID, typeErr := world_types.GetObjectType(ctx, src, objectKey)
	if typeErr != nil {
		return nil, errors.Wrap(typeErr, "get source object type")
	}
	if typeID != "" {
		return nil, errors.Wrapf(err, "get unixfs root constructor for object type %q", typeID)
	}
	return nil, nil
}

func addObjectCopyStats(a, b bucket_lookup.ObjectCopyStats) bucket_lookup.ObjectCopyStats {
	return bucket_lookup.ObjectCopyStats{
		BlocksSeen:         a.BlocksSeen + b.BlocksSeen,
		BlocksCopied:       a.BlocksCopied + b.BlocksCopied,
		BlocksExisting:     a.BlocksExisting + b.BlocksExisting,
		BlocksWritten:      a.BlocksWritten + b.BlocksWritten,
		BlocksDeduped:      a.BlocksDeduped + b.BlocksDeduped,
		SubtreesSkipped:    a.SubtreesSkipped + b.SubtreesSkipped,
		LogicalSourceBytes: a.LogicalSourceBytes + b.LogicalSourceBytes,
	}
}

func ensureCopiedObjectType(ctx context.Context, src, dst world.WorldState, objectKey string) error {
	srcType, err := world_types.GetObjectType(ctx, src, objectKey)
	if err != nil {
		return errors.Wrap(err, "get source object type")
	}
	if srcType == "" {
		return nil
	}

	dstType, err := world_types.GetObjectType(ctx, dst, objectKey)
	if err != nil {
		return errors.Wrap(err, "get destination object type")
	}
	if dstType == srcType {
		return nil
	}
	if dstType != "" {
		return errors.Errorf("destination object %q has type %q, expected %q", objectKey, dstType, srcType)
	}
	return world_types.SetObjectType(ctx, dst, objectKey, srcType)
}

// readCdnV86Image loads the V86Image block from =ws= at =objKey=, verifying the
// object exists and carries the V86Image type marker.
func readCdnV86Image(ctx context.Context, ws world.WorldState, objKey string) (*s4wave_vm.V86Image, error) {
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}
	if !found {
		return nil, errors.Errorf("v86 image object %q not found", objKey)
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return nil, errors.Wrap(err, "get object type")
	}
	if typeID != s4wave_vm.V86ImageTypeID && typeID != legacySpacewaveV86ImageTypeID {
		return nil, errors.Errorf("object %q is not a V86Image (type=%q)", objKey, typeID)
	}

	var img *s4wave_vm.V86Image
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		current, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.V86Image](ctx, bcs, func() block.Block {
			return &s4wave_vm.V86Image{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if current == nil {
			return errors.New("v86 image block missing on object")
		}
		img = current.CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "access v86 image block")
	}
	return img, nil
}

// readV86ImageEdges reads each of the five V86Image asset edges from =ws=,
// returning a map of predicate to target object key. Missing edges are
// represented as empty-string entries in the returned map.
func readV86ImageEdges(ctx context.Context, ws world.WorldState, objKey string) (map[string]string, error) {
	out := make(map[string]string, len(v86ImageEdgePreds))
	for _, pred := range v86ImageEdgePreds {
		target, err := lookupV86ImageEdge(ctx, ws, objKey, pred)
		if err != nil {
			return nil, err
		}
		out[pred] = target
	}
	return out, nil
}

// lookupV86ImageEdge returns the target object key for a single (subject,
// predicate) pair. Returns "" when no quad exists for that edge.
func lookupV86ImageEdge(ctx context.Context, ws world.WorldState, subject, pred string) (string, error) {
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(subject, pred, "", ""),
		1,
	)
	if err != nil {
		return "", errors.Wrapf(err, "lookup %s edge", pred)
	}
	if len(quads) == 0 {
		return "", nil
	}
	target, err := world.GraphValueToKey(quads[0].GetObj())
	if err != nil {
		return "", errors.Wrapf(err, "parse %s edge target key", pred)
	}
	return target, nil
}

// checkDstV86Image verifies that the destination accepts the image copy. A
// matching image is a resumable retry; a different existing object is never
// overwritten.
func checkDstV86Image(
	ctx context.Context,
	dst world.WorldState,
	dstObjectKey string,
	srcImage *s4wave_vm.V86Image,
) (bool, error) {
	if dst.GetReadOnly() {
		return false, errors.New("destination world state is read-only")
	}
	_, found, err := dst.GetObject(ctx, dstObjectKey)
	if err != nil {
		return false, errors.Wrap(err, "probe destination object")
	}
	if !found {
		return false, nil
	}
	dstImage, err := readCdnV86Image(ctx, dst, dstObjectKey)
	if err != nil {
		return false, errors.Wrap(err, "read existing destination image")
	}
	if !dstImage.EqualVT(srcImage) {
		return false, errors.Errorf("destination object %q already contains a different V86Image", dstObjectKey)
	}
	return true, nil
}
