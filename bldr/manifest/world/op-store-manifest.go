package bldr_manifest_world

import (
	"context"

	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ExStoreManifestOp stores a manifest to an object key and links it from
// linkObjKeys. A ManifestRef carrying a release authorization is stored as a
// ManifestRef object, any other as a Manifest object. It applies no operation
// when the World already holds both.
func ExStoreManifestOp(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objectKey string,
	linkObjKeys []string,
	manifestRef *manifest.ManifestRef,
) error {
	// Skip the World write when the manifest and its links are already stored.
	op := NewStoreManifestOp(
		objectKey,
		linkObjKeys,
		manifestRef,
	)
	changes, err := op.ChangesWorld(ctx, ws)
	if err != nil || !changes {
		return err
	}

	// Store the manifest and link it.
	_, _, err = ws.ApplyWorldOp(ctx, op, sender)
	return err
}

// ChangesWorld reports whether applying the op would change ws. It reads ws
// only, so a store of an already stored manifest needs no write transaction.
func (o *StoreManifestOp) ChangesWorld(ctx context.Context, ws world.WorldState) (bool, error) {
	// Check the manifest object and its root ref.
	ref, err := o.resolveManifestRef(ctx, ws)
	if err != nil {
		return false, err
	}
	if ref.GetReleaseAuthorization() != nil {
		stored, err := checkManifestRefStored(ctx, ws, o.GetObjectKey(), ref)
		if err != nil || !stored {
			return !stored, err
		}
	} else {
		obj, objOk, err := ws.GetObject(ctx, o.GetObjectKey())
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return false, err
		}
		if !objOk {
			return true, nil
		}
		replace, err := manifestObjectReplaces(ctx, ws, o.GetObjectKey(), obj, ref.GetManifestRef())
		if err != nil || replace {
			return replace, err
		}
	}

	// Check each link from a linked object to the manifest.
	manifestID := o.GetManifestRef().GetMeta().GetManifestId()
	for _, objKey := range o.GetLinkObjectKeys() {
		quads, err := ws.LookupGraphQuads(ctx, NewManifestQuad(objKey, o.GetObjectKey(), manifestID), 1)
		if err != nil {
			return false, err
		}
		if len(quads) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// NewStoreManifestOp constructs a new StoreManifestOp block.
func NewStoreManifestOp(
	objectKey string,
	linkObjectKeys []string,
	manifestRef *manifest.ManifestRef,
) *StoreManifestOp {
	return &StoreManifestOp{
		ObjectKey:      objectKey,
		LinkObjectKeys: linkObjectKeys,
		ManifestRef:    manifestRef.CloneVT(),
	}
}

// Validate performs cursory checks on the op.
func (o *StoreManifestOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetManifestRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *StoreManifestOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Store the manifest object, or the authorized reference to it.
	ref, err := o.resolveManifestRef(ctx, ws)
	if err != nil {
		return false, err
	}
	var changed bool
	if ref.GetReleaseAuthorization() != nil {
		changed, err = SetManifestRef(ctx, ws, o.GetObjectKey(), ref)
	} else {
		_, changed, err = SetManifest(ctx, ws, sender, o.GetObjectKey(), ref.GetManifestRef())
	}
	if err != nil {
		return false, err
	}

	// link any LinkObjectKeys
	for _, objKey := range o.GetLinkObjectKeys() {
		quad := NewManifestQuad(objKey, o.GetObjectKey(), o.GetManifestRef().GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return false, err
		}
	}

	// increment version of the linked objects if the manifest changed
	if changed {
		for _, objKey := range o.GetLinkObjectKeys() {
			// get the object with objKey
			obj, err := world.MustGetObject(ctx, ws, objKey)
			if err != nil {
				world.ReleaseObjectState(obj)
				if err == context.Canceled {
					return false, err
				}
				return false, errors.Wrap(err, objKey)
			}

			_, err = obj.IncrementRev(ctx)
			world.ReleaseObjectState(obj)
			if err != nil {
				return false, err
			}
		}
	}

	return false, nil
}

// resolveManifestRef returns the ManifestRef the op stores at its object key.
//
// A release authorization signs the Manifest root, not the location of its
// blocks. When the op carries none and the object already holds an authorized
// ManifestRef of the same Manifest root, such as when a download relocates the
// manifest into the World bucket, the stored authorization still applies and
// the result keeps it. Otherwise the result is the op's ManifestRef.
func (o *StoreManifestOp) resolveManifestRef(ctx context.Context, ws world.WorldState) (*manifest.ManifestRef, error) {
	// An op that carries an authorization stores it as given.
	ref := o.GetManifestRef()
	if ref.GetReleaseAuthorization() != nil {
		return ref, nil
	}

	// Only an existing ManifestRef object can hold an authorization to keep.
	objKey := o.GetObjectKey()
	objType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil || objType != ManifestRefTypeID {
		return ref, err
	}
	curr, _, err := readManifestRefObject(ctx, ws, objKey)
	if err != nil {
		return nil, err
	}
	if curr.GetReleaseAuthorization() == nil ||
		!ManifestObjectRefsSameExecutable(curr.GetManifestRef(), ref.GetManifestRef()) {
		return ref, nil
	}

	// Keep the stored authorization of the same manifest.
	kept := ref.CloneVT()
	kept.ReleaseAuthorization = curr.GetReleaseAuthorization().CloneVT()
	return kept, nil
}

// GetPayloadRefs retains the manifest bundle for replay after the World unmounts.
func (o *StoreManifestOp) GetPayloadRefs() []*block.BlockRef {
	return []*block.BlockRef{o.GetManifestRef().GetManifestRef().GetRootRef()}
}

// _ is a type assertion
var _ world.PayloadOperation = (*StoreManifestOp)(nil)
