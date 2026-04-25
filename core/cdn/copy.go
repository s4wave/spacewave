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

// vmImageEdgePreds are the five graph predicates that bind a VmImage to its
// UnixFS asset objects. Copy-from-CDN preserves each edge's target object key
// verbatim so the content-addressed blocks the destination Space fetches
// overlap with the CDN block store and dedup holds.
var vmImageEdgePreds = []string{
	string(s4wave_vm.PredVmImageWasm),
	string(s4wave_vm.PredVmImageBiosSeabios),
	string(s4wave_vm.PredVmImageBiosVgabios),
	string(s4wave_vm.PredVmImageKernel),
	string(s4wave_vm.PredVmImageRootfs),
}

// CopyVmImageFromCdn copies a VmImage (metadata block plus the five asset
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
func CopyVmImageFromCdn(
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

	img, err := readCdnVmImage(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read vm image %q from cdn", srcObjectKey)
	}

	edges, err := readVmImageEdges(ctx, src, srcObjectKey)
	if err != nil {
		return errors.Wrapf(err, "read vm image edges for %q", srcObjectKey)
	}

	if err := checkDstWritable(ctx, dst, dstObjectKey); err != nil {
		return errors.Wrap(err, "destination write check")
	}

	createdAt := img.GetCreatedAt().AsTime()
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	op := s4wave_vm.NewCreateVmImageOp(dstObjectKey, img, createdAt)
	if _, _, err := dst.ApplyWorldOp(ctx, op, ""); err != nil {
		return errors.Wrap(err, "apply create vm image op on destination")
	}

	for pred, targetKey := range edges {
		if targetKey == "" {
			continue
		}
		quad := world.NewGraphQuadWithKeys(dstObjectKey, pred, targetKey, "")
		if err := dst.SetGraphQuad(ctx, quad); err != nil {
			return errors.Wrapf(err, "set %s edge on destination", pred)
		}
	}

	return nil
}

// readCdnVmImage loads the VmImage block from =ws= at =objKey=, verifying the
// object exists and carries the VmImage type marker.
func readCdnVmImage(ctx context.Context, ws world.WorldState, objKey string) (*s4wave_vm.VmImage, error) {
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}
	if !found {
		return nil, errors.Errorf("vm image object %q not found", objKey)
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		return nil, errors.Wrap(err, "get object type")
	}
	if typeID != s4wave_vm.VmImageTypeID {
		return nil, errors.Errorf("object %q is not a VmImage (type=%q)", objKey, typeID)
	}

	var img *s4wave_vm.VmImage
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		current, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmImage](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmImage{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if current == nil {
			return errors.New("vm image block missing on object")
		}
		img = current.CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "access vm image block")
	}
	return img, nil
}

// readVmImageEdges reads each of the five VmImage asset edges from =ws=,
// returning a map of predicate to target object key. Missing edges are
// represented as empty-string entries in the returned map.
func readVmImageEdges(ctx context.Context, ws world.WorldState, objKey string) (map[string]string, error) {
	out := make(map[string]string, len(vmImageEdgePreds))
	for _, pred := range vmImageEdgePreds {
		target, err := lookupVmImageEdge(ctx, ws, objKey, pred)
		if err != nil {
			return nil, err
		}
		out[pred] = target
	}
	return out, nil
}

// lookupVmImageEdge returns the target object key for a single (subject,
// predicate) pair. Returns "" when no quad exists for that edge.
func lookupVmImageEdge(ctx context.Context, ws world.WorldState, subject, pred string) (string, error) {
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
// VmImage at =dstObjectKey=. Fails loud when =dst= is explicitly read-only
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
