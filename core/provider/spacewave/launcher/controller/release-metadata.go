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
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

const (
	releaseWorldEngineID              = "spacewave-release-world"
	releaseMetadataDirectoryObjectKey = "release/metadata"
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
		return nil
	}
	metadata, err := c.resolveReleaseMetadata(ctx, distConf.ResolvedChannelKey())
	if err != nil {
		c.setUpdateError(err)
		return err
	}
	platformID, err := nativeDesktopPlatformID()
	if err != nil {
		c.setUpdateError(err)
		return err
	}
	manifestRef := selectReleaseManifestRef(metadata, platformID)
	if err := c.stageReleaseManifestUpdate(ctx, metadata, platformID, manifestRef); err != nil {
		c.setUpdateError(err)
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
	var metadata *spacewave_release.ReleaseMetadata
	err = world.ExecTransaction(ctx, eng, false, func(ctx context.Context, wtx world.WorldState) error {
		var readErr error
		metadata, readErr = readSelectedReleaseMetadata(ctx, wtx, channelKey)
		return readErr
	})
	return metadata, err
}

func (c *Controller) setUpdateError(err error) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		info.UpdateState = &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
			ErrorMessage: err.Error(),
		}
		return true, nil
	})
}

func (c *Controller) clearMatchingUpdateError() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_ERROR {
			return false, nil
		}
		info.UpdateState = nil
		return true, nil
	})
}

func (c *Controller) clearUpdateState() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.UpdateState == nil {
			return false, nil
		}
		info.UpdateState = nil
		return true, nil
	})
}

func (c *Controller) stageReleaseManifestUpdate(
	ctx context.Context,
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
	manifestRef *bldr_manifest.ManifestRef,
) error {
	if manifestRef == nil {
		return errors.New("release metadata does not support platform " + platformID)
	}
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
	if err := verifyStagedReleaseEntrypoint(ctx, stageRoot, stagedPath); err != nil {
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
}

func verifyStagedReleaseEntrypoint(ctx context.Context, stageRoot, stagedPath string) error {
	stagedInfo, err := os.Stat(stagedPath)
	if err != nil {
		return errors.Wrap(err, "stat staged release entrypoint")
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
	return selectReleaseManifestRef(metadata, platformID) != nil
}

func selectReleaseManifestRef(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
) *bldr_manifest.ManifestRef {
	for _, ref := range metadata.GetManifestRefs() {
		if ref.GetMeta().GetPlatformId() == platformID {
			return ref
		}
	}
	return nil
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
