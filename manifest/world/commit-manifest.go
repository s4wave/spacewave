package bldr_manifest_world

import (
	"context"
	"io/fs"

	"github.com/aperturerobotics/bifrost/peer"
	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
)

// CommitManifest commits the manifest with output paths.
func CommitManifest(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFs, assetsFs fs.FS,
	manifestObjKey string,
	linkObjKeys []string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	var out *manifest.Manifest
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) (err error) {
		out, err = manifest.CreateManifest(
			ctx,
			bcs,
			meta,
			entrypointFilename,
			distFs,
			assetsFs,
			ts,
		)
		return err
	})
	if err != nil {
		return nil, manifestRef, err
	}

	le.
		WithField("object-key", manifestObjKey).
		WithField("link-object-keys", linkObjKeys).
		Infof("committing manifest to world: %s", manifestRef.MarshalString())
	_, _, err = ws.ApplyWorldOp(
		NewStoreManifestOp(
			manifestObjKey,
			linkObjKeys,
			manifest.NewManifestRef(
				out.GetMeta(),
				manifestRef,
			),
		),
		opPeerID,
	)
	if err != nil {
		return nil, manifestRef, err
	}
	return out, manifestRef, nil
}
