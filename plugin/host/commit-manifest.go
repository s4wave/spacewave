package plugin_host

import (
	"context"
	"io/fs"

	"github.com/aperturerobotics/bifrost/peer"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
)

// CommitPluginManifest commits the plugin manifest with output paths.
func CommitPluginManifest(
	ctx context.Context,
	le *logrus.Entry,
	engine world.Engine,
	meta *plugin.PluginManifestMeta,
	entrypointFilename string,
	distFs, assetsFs fs.FS,
	manifestObjKey string,
	linkObjKeys []string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*plugin.PluginManifest, *bucket.ObjectRef, error) {
	var manifest *plugin.PluginManifest
	manifestRef, err := world.AccessObject(ctx, engine.AccessWorldState, nil, func(bcs *block.Cursor) (err error) {
		manifest, err = plugin.CreatePluginManifest(
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

	le.Infof("committing plugin manifest to world: %s", manifestRef.MarshalString())
	tx, err := engine.NewTransaction(true)
	if err != nil {
		return nil, manifestRef, err
	}
	defer tx.Discard()

	_, _, err = tx.ApplyWorldOp(
		NewStorePluginManifestOp(
			manifestObjKey,
			linkObjKeys,
			manifest.GetMeta(),
			manifestRef,
		),
		opPeerID,
	)
	if err != nil {
		return nil, manifestRef, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, manifestRef, err
	}

	return manifest, manifestRef, nil
}
