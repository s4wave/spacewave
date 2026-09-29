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
	return StoreManifestBundles(
		ctx,
		ws,
		sender,
		tuple.GetObjectKey(),
		tuple.GetLinkObjectKeys(),
		[]*bldr_manifest.ManifestRef{manifestRef},
		ts,
	)
}

// StoreManifestBundles stores manifest refs under one ManifestBundle root at
// bundleKey and links the bundle from each link key.
func StoreManifestBundles(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	bundleKey string,
	linkObjKeys []string,
	manifestRefs []*bldr_manifest.ManifestRef,
	ts *timestamppb.Timestamp,
) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	// Default the bundle timestamp to now.
	if ts == nil {
		ts = timestamppb.Now()
	}

	// Store each Manifest under its bundle entry key.
	manifestObjKeys := make([]string, len(manifestRefs))
	for i, manifestRef := range manifestRefs {
		if err := manifestRef.Validate(); err != nil {
			return nil, nil, err
		}
		manifestObjKey, err := bldr_manifest.NewManifestBundleEntryKey(bundleKey, manifestRef.GetMeta())
		if err != nil {
			return nil, nil, err
		}
		_, _, err = bldr_manifest_world.SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			return nil, nil, errors.Wrap(err, "store manifest")
		}
		manifestObjKeys[i] = manifestObjKey
	}

	// Create the bundle object over the stored Manifests.
	bundle, bundleRef, err := bldr_manifest_world.CreateManifestBundle(ctx, ws, bundleKey, manifestObjKeys, ts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create manifest bundle")
	}

	// Link the bundle from each link store.
	for _, objKey := range linkObjKeys {
		if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
			return nil, nil, errors.Wrap(err, "create manifest link store")
		}
		quad := bldr_manifest_world.NewManifestQuad(objKey, bundleKey, "")
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return nil, nil, errors.Wrap(err, "link manifest bundle")
		}
	}
	return bundle, bundleRef, nil
}
