package bldr_manifest_pack

import (
	"context"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// StoreManifestBundle stores one fetched manifest ref under a ManifestBundle root.
func StoreManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	tuple *ManifestTuple,
	manifestRef *bldr_manifest.ManifestRef,
	ts *timestamppb.Timestamp,
) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	if err := tuple.Validate(); err != nil {
		return nil, nil, err
	}
	if err := manifestRef.Validate(); err != nil {
		return nil, nil, err
	}
	if ts == nil {
		ts = timestamppb.Now()
	}
	manifestObjKey, err := bldr_manifest.NewManifestBundleEntryKey(tuple.GetObjectKey(), manifestRef.GetMeta())
	if err != nil {
		return nil, nil, err
	}
	_, _, err = bldr_manifest_world.SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
	if err != nil {
		return nil, nil, errors.Wrap(err, "store manifest")
	}
	bundle, bundleRef, err := bldr_manifest_world.CreateManifestBundle(
		ctx,
		ws,
		tuple.GetObjectKey(),
		[]string{manifestObjKey},
		ts,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create manifest bundle")
	}
	for _, objKey := range tuple.GetLinkObjectKeys() {
		quad := bldr_manifest_world.NewManifestQuad(objKey, tuple.GetObjectKey(), "")
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return nil, nil, errors.Wrap(err, "link manifest bundle")
		}
	}
	return bundle, bundleRef, nil
}
