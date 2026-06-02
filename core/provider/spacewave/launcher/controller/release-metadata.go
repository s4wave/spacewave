//go:build !js

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

const (
	releaseWorldEngineID              = "spacewave-release-world"
	releaseMetadataDirectoryObjectKey = "release/metadata"
	nativeEntrypointManifestID        = "spacewave-dist"
	cliEntrypointManifestID           = "spacewave-cli"
)

func (c *Controller) refreshCurrentReleaseMetadataStatus(ctx context.Context) error {
	info, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	return c.refreshReleaseMetadataStatus(ctx, info.GetDistConfig())
}

// refreshReleaseMetadataStatus resolves release metadata for the current DistConfig.
func (c *Controller) refreshReleaseMetadataStatus(ctx context.Context, distConf *spacewave_launcher.DistConfig) error {
	if distConf.GetRev() == 0 {
		c.clearUpdateState()
		c.setSelectedEntrypointManifestRef(nil)
		c.setReleaseMetadataOutcome("idle")
		return nil
	}
	c.setReleaseMetadataOutcome("resolving")
	c.setSelectedEntrypointManifestRef(nil)
	c.setReleaseWorldHeadRef("")
	metadata, err := c.resolveReleaseMetadata(ctx, distConf.ResolvedChannelKey())
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	platformID, err := nativeDesktopPlatformID()
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	manifestRef, err := selectReleaseManifestRef(metadata, platformID)
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	if err := c.stageReleaseManifestUpdate(ctx, metadata, platformID, manifestRef); err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	return nil
}

func (c *Controller) resolveReleaseMetadata(
	ctx context.Context,
	channelKey string,
) (*spacewave_release.ReleaseMetadata, error) {
	eng, _, ref, err := world.ExLookupWorldEngine(ctx, c.bus, true, releaseWorldEngineID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup release world")
	}
	if eng == nil || ref == nil {
		return nil, errors.New("release world not mounted")
	}
	defer ref.Release()
	metadata, headRef, err := readReleaseMetadataSnapshot(ctx, c.le, eng, channelKey)
	if err != nil {
		return nil, err
	}
	c.setReleaseWorldHeadRef(headRef)
	return metadata, err
}

func (c *Controller) stageReleaseManifestUpdate(
	ctx context.Context,
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
	manifestRef *bldr_manifest.ManifestRef,
) error {
	if manifestRef == nil {
		return errors.New("release metadata missing native entrypoint manifest " + nativeEntrypointManifestID + " for platform " + platformID)
	}
	c.setSelectedEntrypointManifestRef(manifestRef)
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return errors.Wrap(err, "get staging dir")
	}
	stageRoot := filepath.Join(stagingDir, metadata.GetVersion())
	distPath := filepath.Join(stageRoot, "dist")
	assetsPath := filepath.Join(stageRoot, "assets")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return errors.Wrap(err, "create update staging dir")
	}
	c.setUpdateDownloading(metadata.GetVersion())

	eng, _, ref, err := world.ExLookupWorldEngine(ctx, c.bus, true, releaseWorldEngineID, nil)
	if err != nil {
		return errors.Wrap(err, "lookup release world")
	}
	defer ref.Release()

	var stagedPath string
	err = world.ExecTransaction(ctx, eng, false, func(ctx context.Context, wtx world.WorldState) error {
		manifest, err := checkoutReleaseManifest(ctx, c.le, wtx, manifestRef, distPath, assetsPath)
		if err != nil {
			return err
		}
		stagedPath = filepath.Join(distPath, manifest.GetEntrypoint())
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "checkout release manifest")
	}
	if err := c.verifyStagedReleaseEntrypoint(ctx, platformID, stageRoot, stagedPath); err != nil {
		return err
	}
	c.setUpdateStaged(metadata.GetVersion(), stagedPath)
	return nil
}

func (c *Controller) resolveStagingDir() (string, error) {
	if c.stagingDirFunc != nil {
		return c.stagingDirFunc()
	}
	return getStagingDir()
}

func (c *Controller) setUpdateDownloading(version string) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		info.UpdateState = &spacewave_launcher.UpdateState{
			Phase:   spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING,
			Version: version,
		}
		return true, nil
	})
	c.setReleaseMetadataOutcome("downloading")
}

func (c *Controller) setUpdateStaged(version, stagedPath string) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		info.UpdateState = &spacewave_launcher.UpdateState{
			Phase:            spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
			Version:          version,
			DownloadProgress: 100,
			StagedPath:       stagedPath,
		}
		return true, nil
	})
	c.setReleaseMetadataOutcome("staged")
}

func (c *Controller) setReleaseMetadataOutcome(outcome string) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.ReleaseMetadataOutcome = outcome
	})
}

func (c *Controller) setReleaseWorldHeadRef(ref string) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.ReleaseWorldHeadRef = ref
	})
}

func (c *Controller) setSelectedEntrypointManifestRef(ref *bldr_manifest.ManifestRef) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.SelectedEntrypointManifestID = ""
		next.SelectedEntrypointPlatformID = ""
		next.SelectedEntrypointManifestRev = 0
		next.SelectedEntrypointManifestRef = ""
		if ref == nil {
			return
		}
		next.SelectedEntrypointManifestID = ref.GetMeta().GetManifestId()
		next.SelectedEntrypointPlatformID = ref.GetMeta().GetPlatformId()
		next.SelectedEntrypointManifestRev = ref.GetMeta().GetRev()
		next.SelectedEntrypointManifestRef = ref.GetManifestRef().MarshalString()
	})
}

func (c *Controller) verifyStagedReleaseEntrypoint(
	ctx context.Context,
	platformID string,
	stageRoot string,
	stagedPath string,
) error {
	stagedInfo, err := os.Stat(stagedPath)
	if err != nil {
		return errors.Wrap(err, "stat staged release entrypoint")
	}
	if isDarwinDesktopPlatform(platformID) {
		if err := c.verifyDarwinInstalledAppStagedEntrypoint(stageRoot, stagedPath, stagedInfo.IsDir()); err != nil {
			return err
		}
	}
	if !stagedInfo.IsDir() {
		return nil
	}
	if !strings.HasSuffix(stagedPath, ".app") {
		_ = os.RemoveAll(stageRoot)
		return errors.New("staged directory entrypoint must be a .app bundle")
	}
	if err := verifyAppBundleCodesign(ctx, stagedPath); err != nil {
		_ = os.RemoveAll(stageRoot)
		return errors.Wrap(err, "verify staged app bundle")
	}
	return nil
}

func (c *Controller) verifyDarwinInstalledAppStagedEntrypoint(
	stageRoot string,
	stagedPath string,
	stagedIsDir bool,
) error {
	_, isBundle, _, err := c.currentExecutableBundle()
	if err != nil {
		return err
	}
	if !isBundle {
		return nil
	}
	if stagedIsDir && strings.HasSuffix(stagedPath, ".app") {
		return nil
	}
	_ = os.RemoveAll(stageRoot)
	return errors.New("darwin installed-app update must stage a signed .app bundle")
}

func readReleaseMetadataSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	eng world.Engine,
	channelKey string,
) (*spacewave_release.ReleaseMetadata, string, error) {
	var metadata *spacewave_release.ReleaseMetadata
	var headRef string
	err := eng.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		headRef = root.GetRef().MarshalString()
		ws, err := world_block.BuildWorldStateFromCursor(
			ctx,
			le,
			false,
			root,
			eng,
			bldr_manifest_world.LookupOp,
			false,
		)
		if err != nil {
			return err
		}
		metadata, err = readSelectedReleaseMetadata(ctx, ws, channelKey)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return metadata, headRef, nil
}

func readSelectedReleaseMetadata(
	ctx context.Context,
	ws world.WorldState,
	channelKey string,
) (*spacewave_release.ReleaseMetadata, error) {
	if channelKey == "" {
		return nil, errors.New("release channel key is empty")
	}
	directory, err := readReleaseMetadataBlock[*spacewave_release.ChannelDirectory](
		ctx,
		ws,
		releaseMetadataDirectoryObjectKey,
		func() block.Block { return &spacewave_release.ChannelDirectory{} },
	)
	if err != nil {
		return nil, errors.Wrap(err, "read release channel directory")
	}
	if err := directory.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate release channel directory")
	}
	var metadataRef *block.BlockRef
	for _, entry := range directory.GetChannels() {
		if entry.GetChannelKey() == channelKey {
			metadataRef = entry.GetReleaseMetadataRef()
			break
		}
	}
	if metadataRef == nil || metadataRef.GetEmpty() {
		return nil, errors.New("release metadata missing for channel " + channelKey)
	}
	metadata, err := readReleaseMetadataBlock[*spacewave_release.ReleaseMetadata](
		ctx,
		ws,
		releaseMetadataObjectKey(channelKey),
		func() block.Block { return &spacewave_release.ReleaseMetadata{} },
	)
	if err != nil {
		return nil, errors.Wrap(err, "read release metadata for channel "+channelKey)
	}
	if err := metadata.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate release metadata")
	}
	if metadata.GetChannelKey() != channelKey {
		return nil, errors.New("release metadata channel key mismatch")
	}
	return metadata, nil
}

func readReleaseMetadataBlock[T block.Block](
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	ctor func() block.Block,
) (T, error) {
	obj, err := world.MustGetObject(ctx, ws, objKey)
	var zero T
	if err != nil {
		return zero, err
	}
	var out T
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		blk, err := block.UnmarshalBlock[block.Block](ctx, bcs, ctor)
		if err != nil {
			return err
		}
		typed, ok := blk.(T)
		if !ok {
			return errors.New("release metadata block type mismatch")
		}
		out = typed
		return nil
	})
	return out, err
}

func releaseMetadataObjectKey(channelKey string) string {
	return path.Join(releaseMetadataDirectoryObjectKey, channelKey)
}

func releaseMetadataSupportsPlatform(metadata *spacewave_release.ReleaseMetadata, platformID string) bool {
	_, err := selectReleaseManifestRef(metadata, platformID)
	return err == nil
}

func selectReleaseManifestRef(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
) (*bldr_manifest.ManifestRef, error) {
	return selectReleaseManifestRefByID(metadata, platformID, nativeEntrypointManifestID, "native")
}

func selectCLIReleaseManifestRef(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
) (*bldr_manifest.ManifestRef, error) {
	return selectReleaseManifestRefByID(metadata, platformID, cliEntrypointManifestID, "cli")
}

func selectReleaseManifestRefByID(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
	manifestID string,
	roleName string,
) (*bldr_manifest.ManifestRef, error) {
	var selected *bldr_manifest.ManifestRef
	var nonEntrypoint []string
	for _, ref := range metadata.GetManifestRefs() {
		meta := ref.GetMeta()
		if meta.GetPlatformId() != platformID {
			continue
		}
		if meta.GetManifestId() != manifestID {
			nonEntrypoint = append(nonEntrypoint, meta.GetManifestId())
			continue
		}
		if selected != nil {
			return nil, errors.Errorf(
				"release metadata has duplicate %s entrypoint manifest %s for platform %s",
				roleName,
				manifestID,
				platformID,
			)
		}
		selected = ref
	}
	if selected != nil {
		return selected, nil
	}
	if len(nonEntrypoint) != 0 {
		return nil, errors.Errorf(
			"release metadata has non-entrypoint %s manifests for platform %s (%s), missing %s",
			roleName,
			platformID,
			strings.Join(nonEntrypoint, ", "),
			manifestID,
		)
	}
	return nil, errors.Errorf("release metadata missing %s entrypoint manifest %s for platform %s", roleName, manifestID, platformID)
}

func checkoutReleaseManifest(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestRef *bldr_manifest.ManifestRef,
	distPath string,
	assetsPath string,
) (*bldr_manifest.Manifest, error) {
	return bldr_manifest_world.CheckoutManifest(
		ctx,
		le.WithField("manifest-id", manifestRef.GetMeta().GetManifestId()),
		ws.AccessWorldState,
		manifestRef.GetManifestRef(),
		distPath,
		assetsPath,
		unixfs_sync.DeleteMode_DeleteMode_BEFORE,
		nil,
		nil,
	)
}

func nativeDesktopPlatformID() (string, error) {
	platform, err := bldr_platform.ParseNativePlatform("desktop")
	if err != nil {
		return "", err
	}
	return platform.GetPlatformID(), nil
}

func isDarwinDesktopPlatform(platformID string) bool {
	return strings.HasPrefix(platformID, "desktop/darwin/")
}
