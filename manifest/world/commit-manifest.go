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
	engine world.Engine,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFs, assetsFs fs.FS,
	manifestObjKey string,
	linkObjKeys []string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	var out *manifest.Manifest
	manifestRef, err := world.AccessObject(ctx, engine.AccessWorldState, nil, func(bcs *block.Cursor) (err error) {
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

	le.Infof("committing manifest to world: %s", manifestRef.MarshalString())
	tx, err := engine.NewTransaction(true)
	if err != nil {
		return nil, manifestRef, err
	}
	defer tx.Discard()

	_, _, err = tx.ApplyWorldOp(
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

	if err := tx.Commit(ctx); err != nil {
		return nil, manifestRef, err
	}

	return out, manifestRef, nil
}
