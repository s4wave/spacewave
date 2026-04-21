package bldr_manifest_world

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
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
