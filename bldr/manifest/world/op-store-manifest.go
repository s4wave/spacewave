package bldr_manifest_world

import (
	"context"

	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// StoreManifestOpId is the operation ID for StoreManifest.
var StoreManifestOpId = "bldr/manifest/store"

// ExStoreManifestOp stores a manifest to an object key and links it from
// linkObjKeys. It applies no operation when the World already holds both.
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
	obj, objOk, err := ws.GetObject(ctx, o.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	if !objOk {
		return true, nil
	}
	replace, err := manifestRootRefReplaces(ctx, ws, obj, o.GetManifestRef().GetManifestRef())
	if err != nil || replace {
		return replace, err
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

// NewStoreManifestOpBlock constructs a new StoreManifestOp block.
func NewStoreManifestOpBlock() block.Block {
	return &StoreManifestOp{}
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

// GetOperationTypeId returns the operation type identifier.
func (o *StoreManifestOp) GetOperationTypeId() string {
	return StoreManifestOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *StoreManifestOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// store the object for the manifest
	_, changed, err := SetManifest(ctx, ws, sender, o.GetObjectKey(), o.GetManifestRef().GetManifestRef())
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

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *StoreManifestOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *StoreManifestOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *StoreManifestOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = (*StoreManifestOp)(nil)
