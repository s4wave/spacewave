package dist_entrypoint

import (
	"context"
	"io"
	"io/fs"
	"strings"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
	manifest_fetch_world "github.com/aperturerobotics/bldr/manifest/fetch/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	kvfile_compress "github.com/aperturerobotics/go-kvfile/compress"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_kvfile "github.com/aperturerobotics/hydra/volume/kvfile"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Run builds the bus & starts the dist entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	distMeta *bldr_dist.DistMeta,
	assetsFS fs.FS,
) error {
	if err := distMeta.Validate(); err != nil {
		return errors.Wrap(err, "dist_meta")
	}

	projectID := distMeta.GetProjectId()
	storageRoot, err := DetermineStorageRoot(projectID)
	if err != nil {
		le.WithError(err).Warn("unable to determine storage root, using current dir")
		storageRoot = "./state"
	}

	platformID := distMeta.GetPlatformId()
	distBus, err := BuildDistBus(ctx, le, projectID, platformID, storageRoot)
	if err != nil {
		return errors.Wrap(err, "unable to initialize")
	}
	defer distBus.Release()
	b := distBus.GetBus()

	writeBanner()

	// Create LoadPlugin directives for the startup plugins.
	for _, pluginID := range distMeta.GetStartupPlugins() {
		_, diRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			le.WithError(err).Warn("failed to load startup plugin")
			continue
		}
		defer diRef.Release()
	}

	// fatal error channel
	errCh := make(chan error, 5)

	// mount the config set
	configSetBinFilename := "config-set.bin"
	configSetData, err := fs.ReadFile(assetsFS, configSetBinFilename)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		configSetData = nil
	}

	set := &configset_proto.ConfigSet{}
	if err := set.UnmarshalVT(configSetData); err != nil {
		return err
	}
	cset, err := set.Resolve(ctx, b)
	if err != nil {
		return err
	}
	if len(cset) != 0 {
		_, applyCsetRef, err := b.AddDirective(configset.NewApplyConfigSet(cset), nil)
		if err != nil {
			return err
		}
		defer applyCsetRef.Release()
	}

	// mount the embedded read-only storage volume
	var staticVolFile any
	staticVolFile, err = openStaticVolume(assetsFS)
	if err != nil {
		return errors.Wrap(err, "open static assets volume")
	}

	// call close function if applicable
	if closer, ok := staticVolFile.(io.Closer); ok {
		defer closer.Close()
	}

	staticVolID := "dist-volume"
	staticVolCtrl := NewStaticVolumeController(
		le,
		b,
		staticVolFile.(kvfile_compress.ReadSeekerAt),
		&volume_kvfile.Config{
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias:           []string{staticVolID},
				DisableEventBlockRm:     true,
				DisableReconcilerQueues: true,
				DisablePeer:             true,
			},
		},
		nil,
	)
	relStaticVolCtrl, err := b.AddController(ctx, staticVolCtrl, func(exitErr error) {
		errCh <- exitErr
	})
	if err != nil {
		return errors.Wrap(err, "add static volume controller")
	}
	defer relStaticVolCtrl()

	// mount the manifest kvtx block world backed by read-only storage
	// note: make sure this matches dist compiler at create the embedded manifests world
	distBundleObjKey := "dist"
	embedWorldID := strings.Join([]string{"dist", projectID}, "/")
	embedBucketID := embedWorldID
	embedObjStoreID := embedWorldID
	embedEngineConf := world_block_engine.NewConfig(
		embedWorldID,
		staticVolID,
		embedBucketID,
		embedObjStoreID,
		nil,
		nil,
	)
	_, _, embedEngineCtrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(embedEngineConf),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "start static embedded engine controller")
	}
	defer embedEngineCtrlRef.Release()

	// mount the manifest fetcher from the static world
	staticManifestFetcher := manifest_fetch_world.NewController(le, b, &manifest_fetch_world.Config{
		EngineId:   embedWorldID,
		ObjectKeys: []string{distBundleObjKey},
	})
	relStaticManifestFetcher, err := b.AddController(ctx, staticManifestFetcher, func(exitErr error) {
		errCh <- exitErr
	})
	if err != nil {
		return errors.Wrap(err, "start static manifest fetcher")
	}
	defer relStaticManifestFetcher()

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}
