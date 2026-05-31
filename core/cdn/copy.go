package cdn

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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
	if srcObjectKey == "" {
		return errors.New("source object key is required")
	}
	if dstObjectKey == "" {
		return errors.New("destination object key is required")
	}

	img, err := readCdnV86Image(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read v86 image %q from cdn", srcObjectKey)
	}

	edges, err := readV86ImageEdges(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read v86 image edges for %q", srcObjectKey)
	}

	if err := checkDstWritable(ctx, dst, dstObjectKey); err != nil {
		return errors.Wrap(err, "destination write check")
	}

	createdAt := img.GetCreatedAt().AsTime()
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	op := s4wave_vm.NewCreateV86ImageOp(dstObjectKey, img, createdAt)
	if _, _, err := dst.ApplyWorldOp(ctx, op, ""); err != nil {
		return errors.Wrap(err, "apply create v86 image op on destination")
	}

	for pred, targetKey := range edges {
		if targetKey == "" {
			continue
		}
		if err := ensureCopiedWorldObject(ctx, src, dst, targetKey); err != nil {
			return errors.Wrapf(err, "copy %s asset object %q to destination", pred, targetKey)
		}
		quad := world.NewGraphQuadWithKeys(dstObjectKey, pred, targetKey, "")
		if err := dst.SetGraphQuad(ctx, quad); err != nil {
			return errors.Wrapf(err, "set %s edge on destination", pred)
		}
	}

	return nil
}

// ensureCopiedWorldObject creates objectKey in dst from src when the destination
// does not already have it. V86Image edges point at UnixFS objects, and the
// world graph requires both endpoints to exist before SetGraphQuad can link
// them.
func ensureCopiedWorldObject(ctx context.Context, src, dst world.WorldState, objectKey string) error {
	if objectKey == "" {
		return nil
	}
	if _, found, err := dst.GetObject(ctx, objectKey); err != nil {
		return errors.Wrap(err, "probe destination object")
	} else if !found {
		srcObj, srcFound, err := src.GetObject(ctx, objectKey)
		if err != nil {
			return errors.Wrap(err, "get source object")
		}
		if !srcFound {
			return errors.Errorf("source object %q not found", objectKey)
		}

		_, _, err = world.AccessObjectState(ctx, srcObj, false, func(srcCursor *block.Cursor) error {
			_, _, createErr := world.CreateWorldObject(ctx, dst, objectKey, func(dstCursor *block.Cursor) error {
				srcCursor.CopyToRecursive(dstCursor, true, true)
				return nil
			})
			return createErr
		})
		if err != nil {
			return errors.Wrap(err, "create destination object")
		}
	}

	return ensureCopiedObjectType(ctx, src, dst, objectKey)
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

// checkDstWritable ensures the destination WorldState can accept a new
// V86Image at =dstObjectKey=. Fails loud when =dst= is explicitly read-only
// (e.g. another mounted CDN Space) and refuses to overwrite an existing
// object so the copy stays idempotent from the caller's perspective.
func checkDstWritable(ctx context.Context, dst world.WorldState, dstObjectKey string) error {
	if dst.GetReadOnly() {
		return errors.New("destination world state is read-only")
	}
	_, found, err := dst.GetObject(ctx, dstObjectKey)
	if err != nil {
		return errors.Wrap(err, "probe destination object")
	}
	if found {
		return errors.Errorf("destination object %q already exists", dstObjectKey)
	}
	return nil
}
