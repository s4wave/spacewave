package bldr_project_controller

import (
	"context"
	"path"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/world"
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
	// resultPromise contains the result of the compilation.
	resultPromise *promise.PromiseContainer[*ManifestBuilderResult]
}

// NewManifestBuilderConfig constructs a new ManifestBuilderConfig.
func NewManifestBuilderConfig(manifestID, buildType, platformID, remoteID, objectKey string) *ManifestBuilderConfig {
	return &ManifestBuilderConfig{
		ManifestId: manifestID,
		BuildType:  buildType,
		PlatformId: platformID,
		RemoteId:   remoteID,
		ObjectKey:  objectKey,
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
	if m.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// NewManifestBuilderResult constructs a new ManifestBuilderResult.
func NewManifestBuilderResult(
	builderConf *manifest_builder.BuilderConfig,
	builderRes *manifest_builder.BuilderResult,
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
		c:             c,
		conf:          conf,
		resultPromise: promise.NewPromiseContainer[*ManifestBuilderResult](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *manifestBuilderTracker) execute(ctx context.Context) error {
	t.resultPromise.SetPromise(nil)

	// build remote handle
	remoteRef, err := t.c.AddRemoteRef(t.conf.GetRemoteId())
	if err != nil {
		return err
	}
	defer remoteRef.Release()

	// build world engine handle
	worldEngPtr, err := remoteRef.GetResultPromise().Await(ctx)
	if err != nil {
		return err
	}
	worldEng := *worldEngPtr
	ws := world.NewEngineWorldState(ctx, worldEng, true)

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

	// TODO: what if we specify a platform id like "native" that is interpreted to "native/linux/amd64"
	// TODO: could there be a path collision here?
	buildWorkingPath := path.Join(t.c.c.GetWorkingPath(), "build", manifestID, platformIDPath)
	distSrcPath := path.Join(t.c.c.GetWorkingPath(), "bldr")

	// load plugin config from project config
	manifestConfigs := t.c.c.GetProjectConfig().GetManifests()
	manifestConfig := manifestConfigs[manifestID]

	// determine plugin revision from previous version
	rev := manifestConfig.GetRev()
	platformID := meta.GetPlatformId()
	remoteConf := remoteRef.GetRemoteConfig()
	pluginHostKey := remoteConf.GetObjectKey()

	// collect any existing manifests linked to the obj key
	existingManifests, _, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		manifestID,
		platformID,
		pluginHostKey,
	)
	if err != nil {
		return err
	}
	if len(existingManifests) != 0 {
		existingManifest := existingManifests[0]
		if existingRev := existingManifest.GetRev(); existingRev >= rev {
			rev = existingRev + 1
		}
	}

	// build plugin manifest metadata and builder config
	meta.Rev = rev
	manifestKey := t.conf.GetObjectKey()
	manifestBuilderConf := &manifest_builder.BuilderConfig{
		ManifestMeta:   meta,
		EngineId:       remoteConf.GetEngineId(),
		PeerId:         remoteConf.GetPeerId(),
		ObjectKey:      manifestKey,
		DistSourcePath: distSrcPath,
		WorkingPath:    buildWorkingPath,
		SourcePath:     t.c.c.GetSourcePath(),
	}
	builderConf := manifest_builder_controller.NewConfig(
		manifestBuilderConf,
		manifestConfig.GetBuilder(),
		t.c.c.GetBuildBackoff(),
		t.c.c.GetWatch(),
	)

	ctrlInter, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if err != nil {
		t.resultPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := ctrlInter.(*manifest_builder_controller.Controller)
	if !ok {
		err := errors.New("unexpected controller type for plugin builder controller")
		t.resultPromise.SetResult(nil, err)
		return err
	}

	resultPromise := builderCtrl.GetResultPromise()
	result, err := resultPromise.Await(ctx)
	if err != nil {
		t.resultPromise.SetResult(nil, err)
	}

	t.resultPromise.SetResult(NewManifestBuilderResult(manifestBuilderConf, result), nil)

	// wait for ctx to be canceled
	// this allows the builder controller to resolve FetchPlugin
	<-ctx.Done()
	return nil
}
