package bldr_manifest_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/go-git/go-billy/v5"
	"github.com/sirupsen/logrus"
)

// CommitManifest commits the manifest with output paths.
func CommitManifest(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	access world.AccessWorldStateFunc,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFs, assetsFs billy.Filesystem,
	manifestObjKey string,
	linkObjKeys []string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	var out *manifest.Manifest
	manifestRef, err := world.AccessObject(ctx, access, nil, func(bcs *block.Cursor) (err error) {
		out, err = manifest.CreateManifestWithBilly(
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

	out.Meta.Logger(le).
		WithField("object-key", manifestObjKey).
		WithField("link-object-keys", linkObjKeys).
		Info("committing manifest to world")
	_, _, err = ws.ApplyWorldOp(
		ctx,
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
