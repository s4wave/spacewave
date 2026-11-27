// Package os_release is the contract of an os/release object: the Spacewave
// OS image, authorized by its signer, that the Devices whose os/system node
// binds it run.
package os_release

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
)

// NewRelease returns the release of the image that signed authorizes, after
// checking that signed holds a complete image reference. It does not check
// the signature, which only the pins of a Device can judge.
func NewRelease(signed *peer.SignedMsg) (*Release, error) {
	r := &Release{Authorization: signed}
	if _, err := r.Image(); err != nil {
		return nil, err
	}
	return r, nil
}

// Image returns the image reference the authorization holds, unverified.
func (r *Release) Image() (*bucket.ObjectRef, error) {
	// Decode the signed body as an image reference.
	if r.GetAuthorization() == nil {
		return nil, errors.New("release: no authorization")
	}
	image := &bucket.ObjectRef{}
	if err := image.UnmarshalVT(r.GetAuthorization().GetData()); err != nil {
		return nil, errors.Wrap(err, "release: authorization")
	}

	// Require its root and transform.
	if err := os_image.ValidateImage(image); err != nil {
		return nil, errors.Wrap(err, "release")
	}
	return image, nil
}

// WriteRelease stores r as the release key, creating the object when it is
// missing. Every os/system node that binds key moves its Device to r's image.
func WriteRelease(ctx context.Context, ws world.WorldState, key string, r *Release) error {
	// Refuse a release that references no image.
	if _, err := r.Image(); err != nil {
		return err
	}

	// Create a missing release, typed so a node can tell it from any other
	// object.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		created, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
			bcs.SetBlock(r, true)
			return nil
		})
		world.ReleaseObjectState(created)
		if err != nil {
			return errors.Wrapf(err, "create release %q", key)
		}
		return world_types.SetObjectType(ctx, ws, key, ReleaseTypeID)
	}

	// Replace the root block of an existing release.
	if err := world_types.CheckObjectType(ctx, ws, key, ReleaseTypeID); err != nil {
		return err
	}
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(r, true)
		return nil
	})
	return errors.Wrapf(err, "write release %q", key)
}

// ReadRelease reads the release key. It returns world.ErrObjectNotFound when
// the object is missing and an error when it is not an os/release.
func ReadRelease(ctx context.Context, ws world.WorldState, key string) (*Release, error) {
	// Look up the object and require its type.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Wrapf(world.ErrObjectNotFound, "release %q", key)
	}
	if err := world_types.CheckObjectType(ctx, ws, key, ReleaseTypeID); err != nil {
		return nil, err
	}

	// Decode the release from the object's root block.
	var r *Release
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		r, err = block.UnmarshalBlock[Release](ctx, bcs)
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "read release %q", key)
	}
	if r == nil {
		r = &Release{}
	}
	return r, nil
}
