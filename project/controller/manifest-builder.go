//go:build !js

package bldr_project_controller

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
)

// manifestBuilderTracker tracks a running manifest build controller.
type manifestBuilderTracker struct {
	// c is the controller
	c *Controller
	// conf is the manifest builder config
	conf *ManifestBuilderConfig
	// manifestConf is the manifest config
	manifestConf atomic.Pointer[bldr_project.ManifestConfig]
	// remoteConf is the remote config
	remoteConf atomic.Pointer[bldr_project.RemoteConfig]
	// resultPromiseCtr contains the result of the compilation.
	resultPromiseCtr *promise.PromiseContainer[*ManifestBuilderResult]
}

// NewManifestBuilderConfig constructs a new ManifestBuilderConfig.
func NewManifestBuilderConfig(manifestID, buildType, platformID, remoteID string) *ManifestBuilderConfig {
	return &ManifestBuilderConfig{
		ManifestId: manifestID,
		BuildType:  buildType,
		PlatformId: platformID,
		RemoteId:   remoteID,
	}
}

// NewManifestBuilderConfigWithTargetPlatforms constructs a new ManifestBuilderConfig with target platform IDs.
func NewManifestBuilderConfigWithTargetPlatforms(manifestID, buildType, platformID, remoteID string, targetPlatformIDs []string) *ManifestBuilderConfig {
	return &ManifestBuilderConfig{
		ManifestId:        manifestID,
		BuildType:         buildType,
		PlatformId:        platformID,
		RemoteId:          remoteID,
		TargetPlatformIds: targetPlatformIDs,
	}
}

// UnmarshalManifestBuilderConfigB58 unmarshals a b58 manifest builder config.
func UnmarshalManifestBuilderConfigB58(str string) (*ManifestBuilderConfig, error) {
	m := &ManifestBuilderConfig{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// MarshalB58 marshals the conf to a b58 string.
func (m *ManifestBuilderConfig) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

// Validate validates the config.
func (m *ManifestBuilderConfig) Validate() error {
	if err := bldr_manifest.ValidateManifestID(m.GetManifestId(), false); err != nil {
		return err
	}
	if m.GetPlatformId() == "" {
		return bldr_manifest.ErrEmptyPlatformID
	}
	if m.GetRemoteId() == "" {
		return bldr_project.ErrEmptyRemoteID
	}
	return nil
}

// NewManifestBuilderResult constructs a new ManifestBuilderResult.
func NewManifestBuilderResult(
	builderConf *bldr_manifest_builder.BuilderConfig,
	builderRes *bldr_manifest_builder.BuilderResult,
) *ManifestBuilderResult {
	return &ManifestBuilderResult{
		BuilderConfig: builderConf,
		BuilderResult: builderRes,
	}
}

// newManifestBuilderTracker constructs a new build controller tracker.
func (c *Controller) newManifestBuilderTracker(key string) (keyed.Routine, *manifestBuilderTracker) {
	conf, _ := UnmarshalManifestBuilderConfigB58(key)
	tr := &manifestBuilderTracker{
		c:                c,
		conf:             conf,
		resultPromiseCtr: promise.NewPromiseContainer[*ManifestBuilderResult](),
	}
	return tr.execute, tr
}

// failWithError marks the tracker as failed with an error.
func (t *manifestBuilderTracker) failWithError(err error) {
	t.resultPromiseCtr.SetResult(nil, err)
}

// execute executes the tracker.
func (t *manifestBuilderTracker) execute(ctx context.Context) error {
	t.resultPromiseCtr.SetPromise(nil)

	// build remote handle
	worldEng, remoteRef, err := t.c.WaitRemote(ctx, t.conf.GetRemoteId())
	if err != nil {
		return err
	}
	defer remoteRef.Release()
	t.remoteConf.Store(remoteRef.GetRemoteConfig())

	// set config fields
	meta := bldr_manifest.NewManifestMeta(
		t.conf.GetManifestId(),
		bldr_manifest.BuildType(t.conf.GetBuildType()),
		t.conf.GetPlatformId(),
		0,
	)
	manifestID := meta.GetManifestId()
	if manifestID == "" {
		return bldr_manifest.ErrEmptyManifestID
	}

	// ensure that the platform id is clean
	platformIDPath := path.Clean(meta.GetPlatformId())
	if strings.HasPrefix(platformIDPath, "..") {
		return errors.Errorf("invalid platform id: %s", meta.GetPlatformId())
	}

	// ctrlConf is the current controller config
	ctrlConf := t.c.GetConfig()

	// build paths
	buildWorkingPath := filepath.Join(ctrlConf.GetWorkingPath(), "build", platformIDPath, manifestID)
	distSrcPath := filepath.Join(ctrlConf.GetWorkingPath(), "src")

	// load plugin config from project config
	projectConfig := ctrlConf.GetProjectConfig()
	manifestConfigs := projectConfig.GetManifests()
	manifestConfig := manifestConfigs[manifestID].CloneVT()
	if manifestConfig == nil {
		return bldr_project.ErrManifestConfNotFound
	}

	// set the manifest conf
	t.manifestConf.Store(manifestConfig)

	// determine plugin rev from previous version
	rev := manifestConfig.GetRev()
	platformID := meta.GetPlatformId()
	remoteConf := remoteRef.GetRemoteConfig()
	storeObjKey, storeLinkObjKeys := remoteConf.CleanupLinkObjectKeys()

	tx, err := worldEng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// create the plugin host key if it doesn't exist.
	createdStore, err := bldr_manifest_world.CreateManifestStore(ctx, tx, storeObjKey)
	if err != nil {
		tx.Discard()
		return err
	}

	var existingManifests []*bldr_manifest_world.CollectedManifest
	if createdStore {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	} else {
		existingManifests, _, err = bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			tx,
			manifestID,
			[]string{platformID},
			storeLinkObjKeys...,
		)
		tx.Discard()
		if err != nil {
			return err
		}
	}

	if len(existingManifests) != 0 {
		existingManifest := existingManifests[0]
		if existingRev := existingManifest.GetRev(); existingRev >= rev {
			rev = existingRev + 1
		}
	}

	// build plugin manifest metadata and builder config
	meta.Rev = rev
	manifestKey := bldr_manifest.NewManifestKey(storeObjKey, meta)
	manifestBuilderConf := &bldr_manifest_builder.BuilderConfig{
		ProjectId:         projectConfig.GetId(),
		ManifestMeta:      meta,
		EngineId:          remoteConf.GetEngineId(),
		PeerId:            remoteConf.GetPeerId(),
		ObjectKey:         manifestKey,
		LinkObjectKeys:    storeLinkObjKeys,
		DistSourcePath:    distSrcPath,
		WorkingPath:       buildWorkingPath,
		SourcePath:        ctrlConf.GetSourcePath(),
		TargetPlatformIds: t.conf.GetTargetPlatformIds(),
	}
	builderConf := manifest_builder_controller.NewConfig(
		manifestBuilderConf,
		manifestConfig.GetBuilder(),
		ctrlConf.GetBuildBackoff(),
		ctrlConf.GetWatch(),
	)

	builderCtrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*manifest_builder_controller.Controller](
		ctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if err != nil {
		t.resultPromiseCtr.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	for {
		resultPromiseCtr := builderCtrl.GetResultPromise()
		resultPromise, resultPromiseChanged := resultPromiseCtr.GetPromise()

		if resultPromise != nil {
			result, err := resultPromise.Await(ctx)
			if err != nil {
				t.resultPromiseCtr.SetResult(nil, err)
				return err
			}
			t.resultPromiseCtr.SetResult(NewManifestBuilderResult(manifestBuilderConf, result), nil)
		} else {
			// No result yet.
			t.resultPromiseCtr.SetPromise(nil)
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-resultPromiseChanged:
			// re-check (manifest was rebuilt)
		}
	}

	// TODO: cleanup the working dir?
}
