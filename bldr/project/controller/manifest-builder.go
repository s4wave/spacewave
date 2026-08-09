//go:build !js

package bldr_project_controller

import (
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

const (
	manifestBuildBufferedStoreMaxPendingEntries = 16384
	manifestBuildBufferedStoreMaxPendingBytes   = 512 << 20
	manifestBuildBufferedStoreDrainBatchEntries = 1024
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
	// statusMtx guards status
	statusMtx sync.Mutex
	// status is the latest build status
	status ManifestBuilderStatus
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

type bufferedStoreSettingsTx interface {
	SetBufferedStoreSettings(*block.BufferedStoreSettings)
}

func configureManifestBuildTransactionBuffer(tx world.Tx) {
	bufferedTx, ok := tx.(bufferedStoreSettingsTx)
	if !ok {
		return
	}
	bufferedTx.SetBufferedStoreSettings(&block.BufferedStoreSettings{
		MaxPendingEntries: manifestBuildBufferedStoreMaxPendingEntries,
		MaxPendingBytes:   manifestBuildBufferedStoreMaxPendingBytes,
		DrainBatchEntries: manifestBuildBufferedStoreDrainBatchEntries,
	})
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
	if err := m.GetBuildPolicy().Validate(); err != nil {
		return err
	}
	if m.GetPlatformId() == "" {
		return bldr_manifest.ErrEmptyPlatformID
	}
	if _, err := bldr_platform.ParsePlatform(m.GetPlatformId()); err != nil {
		return errors.Wrap(err, "platform_id")
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
	tr.status = tr.newStatus(ManifestBuilderStatusStateQueued, "queued", "")
	return tr.execute, tr
}

// failWithError marks the tracker as failed with an error.
func (t *manifestBuilderTracker) failWithError(err error) {
	t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "builder removed", err)
	t.resultPromiseCtr.SetResult(nil, err)
}

// SetManifestBuilderLifecycleStatus applies builder-controller lifecycle status.
func (t *manifestBuilderTracker) SetManifestBuilderLifecycleStatus(
	status manifest_builder_controller.ManifestBuilderLifecycleStatus,
) {
	next := t.currentManifestBuilderStatus()
	next.CacheHit = status.CacheHit
	next.FullRebuild = status.FullRebuild
	next.HotRebuild = status.HotRebuild
	next.WatchedFileCount = status.WatchedFileCount
	next.DependencyRebuildReason = status.DependencyRebuildReason
	next.Summary = status.Summary
	next.Error = status.Error
	switch status.State {
	case manifest_builder_controller.ManifestBuilderLifecycleStateQueued:
		next.State = ManifestBuilderStatusStateQueued
	case manifest_builder_controller.ManifestBuilderLifecycleStateRunning:
		next.State = ManifestBuilderStatusStateRunning
	case manifest_builder_controller.ManifestBuilderLifecycleStateDone:
		next.State = ManifestBuilderStatusStateDone
	case manifest_builder_controller.ManifestBuilderLifecycleStateError:
		next.State = ManifestBuilderStatusStateError
	}
	t.storeManifestBuilderStatus(next)
}

// execute executes the tracker.
func (t *manifestBuilderTracker) execute(ctx context.Context) error {
	t.resultPromiseCtr.SetPromise(nil)
	t.setManifestBuilderStatus(ManifestBuilderStatusStateRunning, "resolving remote", nil)

	// build remote handle
	worldEng, remoteRef, err := t.c.WaitRemote(ctx, t.conf.GetRemoteId())
	if err != nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "resolve remote", err)
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
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "validate manifest", bldr_manifest.ErrEmptyManifestID)
		return bldr_manifest.ErrEmptyManifestID
	}

	// ensure that the platform id is clean
	platformIDPath := path.Clean(meta.GetPlatformId())
	if strings.HasPrefix(platformIDPath, "..") {
		err := errors.Errorf("invalid platform id: %s", meta.GetPlatformId())
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "validate platform", err)
		return err
	}

	// ctrlConf is the current controller config
	ctrlConf := t.c.GetConfig()

	// build paths
	buildWorkingPath := filepath.Join(ctrlConf.GetWorkingPath(), "build", platformIDPath, manifestID)
	distSrcPath := filepath.Join(ctrlConf.GetWorkingPath(), "src")

	// load plugin config from project config
	projectConfig := ctrlConf.GetProjectConfig()
	manifestConfig := projectConfig.GetManifests()[manifestID].CloneVT()
	if manifestConfig == nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "load manifest config", bldr_project.ErrManifestConfNotFound)
		return bldr_project.ErrManifestConfNotFound
	}
	if err := applyBuilderConfigOverride(manifestConfig, manifestID, t.conf.GetBuilderConfigOverride()); err != nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "apply builder config override", err)
		return err
	}
	t.manifestConf.Store(manifestConfig)
	meta.Description = manifestConfig.GetDescription()

	// determine plugin rev from previous version
	rev := manifestConfig.GetRev()
	platformID := meta.GetPlatformId()
	remoteConf := remoteRef.GetRemoteConfig()
	storeObjKey, storeLinkObjKeys := remoteConf.CleanupLinkObjectKeys()
	var startupBuilderResult *bldr_manifest_builder.BuilderResult

	tx, err := worldEng.NewTransaction(ctx, true)
	if err != nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "open world transaction", err)
		return err
	}
	configureManifestBuildTransactionBuffer(tx)

	// create the plugin host key if it doesn't exist.
	createdStore, err := bldr_manifest_world.CreateManifestStore(ctx, tx, storeObjKey)
	if err != nil {
		tx.Discard()
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "create manifest store", err)
		return err
	}

	var existingManifests []*bldr_manifest_world.CollectedManifest
	if createdStore {
		if err := tx.Commit(ctx); err != nil {
			t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "commit manifest store", err)
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
		if err != nil {
			tx.Discard()
			t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "collect manifests", err)
			return err
		}
		if len(existingManifests) != 0 {
			existingManifest := existingManifests[0]
			worldBuildResult, _, err := resultworld.LookupManifestBuildResult(ctx, tx, existingManifest.ManifestKey)
			if err != nil && !errors.Is(err, world.ErrObjectNotFound) {
				t.c.le.WithError(err).
					WithField("manifest-id", t.conf.GetManifestId()).
					Warn("failed to load world-backed startup build result, falling back to rebuild")
			}
			if worldBuildResult != nil && !worldBuildResult.GetManifest().EqualVT(existingManifest.Manifest) {
				t.c.le.WithField("manifest-id", t.conf.GetManifestId()).
					Warn("world-backed startup build result manifest mismatch, falling back to rebuild")
				worldBuildResult = nil
			}
			if worldBuildResult != nil && !worldBuildResult.GetManifestRef().GetManifestRef().EqualVT(existingManifest.ManifestRef) {
				t.c.le.WithField("manifest-id", t.conf.GetManifestId()).
					Warn("world-backed startup build result manifest ref mismatch, falling back to rebuild")
				worldBuildResult = nil
			}
			if worldBuildResult != nil {
				startupBuilderResult = worldBuildResult.CloneVT()
				t.c.le.WithFields(logrus.Fields{
					"manifest-id":   t.conf.GetManifestId(),
					"manifest-rev":  startupBuilderResult.GetManifest().GetMeta().GetRev(),
					"build-type":    t.conf.GetBuildType(),
					"platform-id":   t.conf.GetPlatformId(),
					"remote-id":     t.conf.GetRemoteId(),
					"startup-state": "world",
				}).Debug("loaded world-backed startup build result")
			}
			if worldBuildResult == nil {
				startupBuilderResult = bldr_manifest_builder.NewBuilderResult(
					existingManifest.Manifest.CloneVT(),
					existingManifest.ManifestRef.Clone(),
					bldr_manifest_builder.NewInputManifest(nil, nil),
				)
			}
		}
		tx.Discard()
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
		BuildPolicy:       t.conf.GetBuildPolicy().CloneVT(),
	}
	builderConf := manifest_builder_controller.NewConfig(
		manifestBuilderConf,
		manifestConfig.GetBuilder(),
		ctrlConf.GetBuildBackoff(),
		ctrlConf.GetWatch(),
		startupBuilderResult,
	)

	// Resolve webPkg dependencies: if this manifest excludes webPkgs
	// that another manifest provides, watch the provider for rebuilds.
	webPkgDeps := resolveWebPkgDeps(t.c.le, projectConfig.GetManifests())
	if watchIDs := webPkgDeps[manifestID]; len(watchIDs) > 0 {
		builderConf.WatchManifestIds = watchIDs
	}

	t.setManifestBuilderStatus(ManifestBuilderStatusStateRunning, "starting builder controller", nil)
	builderCtrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*manifest_builder_controller.Controller](
		ctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfig(builderConf),
		nil,
	)
	if err != nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateError, "start builder controller", err)
		t.resultPromiseCtr.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()
	builderCtrl.SetManifestBuilderLifecycleSink(t)

	for {
		resultPromiseCtr := builderCtrl.GetResultPromise()
		resultPromise, resultPromiseChanged := resultPromiseCtr.GetPromise()

		if resultPromise != nil {
			result, err := resultPromise.Await(ctx)
			if err != nil {
				t.setManifestBuilderTerminalStatus(ctx, "build failed", err)
				t.resultPromiseCtr.SetResult(nil, err)
				return err
			}
			t.setManifestBuilderStatus(ManifestBuilderStatusStateDone, "build complete", nil)
			t.resultPromiseCtr.SetResult(NewManifestBuilderResult(manifestBuilderConf, result), nil)
		}
		if resultPromise == nil {
			// No result yet.
			t.setManifestBuilderStatus(ManifestBuilderStatusStateQueued, "waiting for builder result", nil)
			t.resultPromiseCtr.SetPromise(nil)
		}

		select {
		case <-ctx.Done():
			t.setManifestBuilderStatus(ManifestBuilderStatusStateCanceled, "build canceled", ctx.Err())
			return context.Canceled
		case <-resultPromiseChanged:
			// re-check (manifest was rebuilt)
		}
	}

	// TODO: cleanup the working dir?
}

// applyBuilderConfigOverride applies a build-scoped builder config override to
// a manifest config. REPLACE semantics: the override's config bytes overwrite
// the manifest's static builder config. The override's controller id is
// ignored because the manifest's declared builder id always wins. If the
// override's Rev is non-zero, it replaces the builder rev. A nil override or
// an override with empty config bytes is a no-op.
func applyBuilderConfigOverride(
	manifestConfig *bldr_project.ManifestConfig,
	manifestID string,
	override *configset_proto.ControllerConfig,
) error {
	if override == nil {
		return nil
	}
	overrideBytes := override.GetConfig()
	if len(overrideBytes) == 0 {
		return nil
	}
	builder := manifestConfig.GetBuilder()
	if builder == nil {
		return errors.Errorf("manifest %s: builder_config_override set but manifest has no builder", manifestID)
	}
	builder.Config = slices.Clone(overrideBytes)
	if overrideRev := override.GetRev(); overrideRev != 0 {
		builder.Rev = overrideRev
	}
	return nil
}

func (t *manifestBuilderTracker) setManifestBuilderStatus(
	state ManifestBuilderStatusState,
	summary string,
	err error,
) {
	next := t.currentManifestBuilderStatus()
	next.State = state
	next.Summary = summary
	if err != nil {
		next.Error = err.Error()
	} else {
		next.Error = ""
	}
	t.storeManifestBuilderStatus(next)
}

func (t *manifestBuilderTracker) setManifestBuilderTerminalStatus(
	ctx context.Context,
	summary string,
	err error,
) {
	if ctx.Err() != nil {
		t.setManifestBuilderStatus(ManifestBuilderStatusStateCanceled, "build canceled", ctx.Err())
		return
	}
	t.setManifestBuilderStatus(ManifestBuilderStatusStateError, summary, err)
}

func (t *manifestBuilderTracker) currentManifestBuilderStatus() ManifestBuilderStatus {
	t.statusMtx.Lock()
	status := t.status
	t.statusMtx.Unlock()
	return status
}

func (t *manifestBuilderTracker) refreshManifestBuilderStatusMeta() {
	next := t.currentManifestBuilderStatus()
	next.BuildTargetIDs = t.c.getManifestBuilderBuildTargets(t.conf.MarshalB58())
	next.TargetPlatformIDs = slices.Clone(t.conf.GetTargetPlatformIds())
	t.storeManifestBuilderStatus(next)
}

func (t *manifestBuilderTracker) storeManifestBuilderStatus(status ManifestBuilderStatus) {
	t.statusMtx.Lock()
	t.status = status
	t.statusMtx.Unlock()
	t.publishManifestBuilderStatus()
}

func (t *manifestBuilderTracker) publishManifestBuilderStatus() {
	status := t.currentManifestBuilderStatus()
	t.c.statusSinkMtx.Lock()
	sink := t.c.manifestBuilderStatusSink
	t.c.statusSinkMtx.Unlock()
	if sink != nil {
		sink.SetManifestBuilderStatus(status)
	}
}

func (t *manifestBuilderTracker) newStatus(
	state ManifestBuilderStatusState,
	summary string,
	errText string,
) ManifestBuilderStatus {
	buildTargetIDs := t.c.getManifestBuilderBuildTargets(t.conf.MarshalB58())
	return ManifestBuilderStatus{
		ID:                t.conf.MarshalB58(),
		BuildTargetIDs:    buildTargetIDs,
		ManifestID:        t.conf.GetManifestId(),
		PlatformID:        t.conf.GetPlatformId(),
		TargetPlatformIDs: slices.Clone(t.conf.GetTargetPlatformIds()),
		BuildType:         t.conf.GetBuildType(),
		RemoteID:          t.conf.GetRemoteId(),
		State:             state,
		Summary:           summary,
		Error:             errText,
	}
}

func (c *Controller) addManifestBuilderBuildTarget(conf *ManifestBuilderConfig, target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	key := conf.MarshalB58()
	c.statusSinkMtx.Lock()
	if !slices.Contains(c.manifestBuilderBuildTargets[key], target) {
		c.manifestBuilderBuildTargets[key] = append(c.manifestBuilderBuildTargets[key], target)
	}
	c.statusSinkMtx.Unlock()
}

func (c *Controller) getManifestBuilderBuildTargets(key string) []string {
	c.statusSinkMtx.Lock()
	targets := slices.Clone(c.manifestBuilderBuildTargets[key])
	c.statusSinkMtx.Unlock()
	return targets
}

// _ is a type assertion
var _ manifest_builder_controller.ManifestBuilderLifecycleSink = (*manifestBuilderTracker)(nil)
