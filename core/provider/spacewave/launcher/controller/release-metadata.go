//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strconv"
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
	managedCLIReleaseSidecarFilename  = "managed-cli-release.json"
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
		c.setSelectedCLIManifestRef(nil, "")
		c.setReleaseMetadataOutcome("idle")
		_ = c.clearManagedCLIReleaseSidecar()
		return nil
	}
	c.setReleaseMetadataOutcome("resolving")
	c.setSelectedEntrypointManifestRef(nil)
	c.setSelectedCLIManifestRef(nil, "")
	c.setReleaseWorldHeadRef("")
	_ = c.clearManagedCLIReleaseSidecar()
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
	cliManifestRef, err := selectCLIReleaseManifestRef(metadata, platformID)
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	if err := c.stageReleaseManifestUpdate(ctx, metadata, platformID, manifestRef, cliManifestRef); err != nil {
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
	cliManifestRef *bldr_manifest.ManifestRef,
) error {
	if manifestRef == nil {
		return errors.New("release metadata missing native entrypoint manifest " + nativeEntrypointManifestID + " for platform " + platformID)
	}
	if cliManifestRef == nil {
		return errors.New("release metadata missing cli entrypoint manifest " + cliEntrypointManifestID + " for platform " + platformID)
	}
	c.setSelectedEntrypointManifestRef(manifestRef)
	c.setSelectedCLIManifestRef(cliManifestRef, "")
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return errors.Wrap(err, "get staging dir")
	}
	stageRoot, err := releaseVersionStagingRoot(stagingDir, metadata.GetVersion())
	if err != nil {
		return err
	}
	distPath := filepath.Join(stageRoot, "dist")
	assetsPath := filepath.Join(stageRoot, "assets")
	cliDistPath := filepath.Join(stageRoot, "cli-dist")
	cliAssetsPath := filepath.Join(stageRoot, "cli-assets")
	if err := prepareReleaseStagingRoot(stagingDir, stageRoot, distPath, assetsPath, cliDistPath, cliAssetsPath); err != nil {
		return errors.Wrap(err, "prepare update staging dir")
	}
	c.setUpdateDownloading(metadata.GetVersion())

	eng, _, ref, err := world.ExLookupWorldEngine(ctx, c.bus, true, releaseWorldEngineID, nil)
	if err != nil {
		return errors.Wrap(err, "lookup release world")
	}
	defer ref.Release()

	var stagedPath string
	var cliStagedPath string
	err = world.ExecTransaction(ctx, eng, false, func(ctx context.Context, wtx world.WorldState) error {
		manifest, err := checkoutReleaseManifest(ctx, c.le, wtx, manifestRef, distPath, assetsPath)
		if err != nil {
			return err
		}
		stagedPath, err = stagedManifestEntrypointPath(distPath, manifest.GetEntrypoint())
		if err != nil {
			return errors.Wrap(err, "resolve release manifest entrypoint")
		}
		cliManifest, err := checkoutReleaseManifest(ctx, c.le, wtx, cliManifestRef, cliDistPath, cliAssetsPath)
		if err != nil {
			return errors.Wrap(err, "checkout cli release manifest")
		}
		cliStagedPath, err = stagedManifestEntrypointPath(cliDistPath, cliManifest.GetEntrypoint())
		if err != nil {
			return errors.Wrap(err, "resolve cli release manifest entrypoint")
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "checkout release manifest")
	}
	if err := c.verifyStagedReleaseEntrypoint(ctx, platformID, stageRoot, stagedPath); err != nil {
		return err
	}
	if err := verifyStagedCLIEntrypoint(stageRoot, cliDistPath, cliStagedPath); err != nil {
		return err
	}
	if err := c.writeManagedCLIReleaseSidecar(stagingDir, metadata, cliManifestRef, cliStagedPath); err != nil {
		return err
	}
	c.setSelectedCLIManifestRef(cliManifestRef, cliStagedPath)
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

func (c *Controller) setSelectedCLIManifestRef(ref *bldr_manifest.ManifestRef, stagedPath string) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.SelectedCLIManifestID = ""
		next.SelectedCLIPlatformID = ""
		next.SelectedCLIManifestRev = 0
		next.SelectedCLIManifestRef = ""
		next.SelectedCLIBinaryPath = ""
		if ref == nil {
			return
		}
		next.SelectedCLIManifestID = ref.GetMeta().GetManifestId()
		next.SelectedCLIPlatformID = ref.GetMeta().GetPlatformId()
		next.SelectedCLIManifestRev = ref.GetMeta().GetRev()
		next.SelectedCLIManifestRef = ref.GetManifestRef().MarshalString()
		next.SelectedCLIBinaryPath = stagedPath
	})
}

func stagedManifestEntrypointPath(distPath string, entrypoint string) (string, error) {
	clean := path.Clean(entrypoint)
	if clean == "." || clean == ".." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\\") {
		return "", errors.New("manifest entrypoint must be a local relative path")
	}
	stagedPath := filepath.Join(distPath, filepath.FromSlash(clean))
	rel, err := filepath.Rel(distPath, stagedPath)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("manifest entrypoint escapes staged dist root")
	}
	return stagedPath, nil
}

func releaseVersionStagingRoot(stagingDir string, version string) (string, error) {
	clean := path.Clean(version)
	if clean == "." || clean == ".." || clean != version || path.IsAbs(clean) ||
		strings.Contains(clean, "/") || strings.Contains(version, "\\") {
		return "", errors.New("release version must be a local path segment")
	}
	stageRoot := filepath.Join(stagingDir, filepath.FromSlash(clean))
	if err := verifyPathInsideRoot(stagingDir, stageRoot, "release staging root"); err != nil {
		return "", err
	}
	return stageRoot, nil
}

func prepareReleaseStagingRoot(stagingDir string, stageRoot string, checkoutRoots ...string) error {
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return errors.Wrap(err, "create update staging dir")
	}
	if err := rejectExistingNonDirectoryOrSymlink(stageRoot, "release staging root"); err != nil {
		return err
	}
	if err := os.Mkdir(stageRoot, 0o755); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "create update version staging dir")
	}
	if err := requireDirectoryNotSymlink(stageRoot, "release staging root"); err != nil {
		return err
	}
	for _, root := range checkoutRoots {
		if err := verifyPathInsideRoot(stageRoot, root, "release checkout root"); err != nil {
			return err
		}
		if err := rejectExistingNonDirectoryOrSymlink(root, "release checkout root"); err != nil {
			return err
		}
	}
	return nil
}

func verifyStagedCLIEntrypoint(stageRoot string, cliDistPath string, stagedPath string) error {
	if err := requireDirectoryNotSymlink(stageRoot, "release staging root"); err != nil {
		_ = os.RemoveAll(stageRoot)
		return err
	}
	if err := requireDirectoryNotSymlink(cliDistPath, "staged cli dist root"); err != nil {
		_ = os.RemoveAll(stageRoot)
		return err
	}
	if err := verifyNoSymlinkPath(stageRoot, stagedPath); err != nil {
		_ = os.RemoveAll(stageRoot)
		return err
	}
	if err := verifyNoSymlinkPath(cliDistPath, stagedPath); err != nil {
		_ = os.RemoveAll(stageRoot)
		return err
	}
	stagedInfo, err := os.Lstat(stagedPath)
	if err != nil {
		return errors.Wrap(err, "stat staged cli entrypoint")
	}
	if stagedInfo.Mode()&os.ModeSymlink != 0 {
		_ = os.RemoveAll(stageRoot)
		return errors.New("staged cli entrypoint must not be a symlink")
	}
	if stagedInfo.IsDir() {
		_ = os.RemoveAll(stageRoot)
		return errors.New("staged cli entrypoint must be a file")
	}
	if !stagedInfo.Mode().IsRegular() {
		_ = os.RemoveAll(stageRoot)
		return errors.New("staged cli entrypoint must be a regular file")
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return errors.Wrap(err, "chmod staged cli entrypoint")
	}
	return nil
}

func verifyPathInsideRoot(rootPath string, filePath string, label string) error {
	rel, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New(label + " escapes root")
	}
	return nil
}

func rejectExistingNonDirectoryOrSymlink(dir string, label string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "stat "+label)
	}
	return validateDirectoryNotSymlink(info, label)
}

func requireDirectoryNotSymlink(dir string, label string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return errors.Wrap(err, "stat "+label)
	}
	return validateDirectoryNotSymlink(info, label)
}

func validateDirectoryNotSymlink(info os.FileInfo, label string) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New(label + " must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New(label + " must be a directory")
	}
	return nil
}

func verifyNoSymlinkPath(rootPath string, filePath string) error {
	rel, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("staged cli entrypoint escapes cli dist root")
	}
	dir := rootPath
	elems := strings.Split(rel, string(filepath.Separator))
	for _, elem := range elems[:len(elems)-1] {
		if elem == "" || elem == "." {
			continue
		}
		dir = filepath.Join(dir, elem)
		info, err := os.Lstat(dir)
		if err != nil {
			return errors.Wrap(err, "stat staged cli entrypoint parent")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("staged cli entrypoint parent must not be a symlink")
		}
		if !info.IsDir() {
			return errors.New("staged cli entrypoint parent must be a directory")
		}
	}
	return nil
}

func (c *Controller) clearManagedCLIReleaseSidecar() error {
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(stagingDir, managedCLIReleaseSidecarFilename))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c *Controller) writeManagedCLIReleaseSidecar(
	stagingDir string,
	metadata *spacewave_release.ReleaseMetadata,
	ref *bldr_manifest.ManifestRef,
	binaryPath string,
) error {
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return errors.Wrap(err, "create managed cli release sidecar dir")
	}
	sidecarPath := filepath.Join(stagingDir, managedCLIReleaseSidecarFilename)
	tempPath := sidecarPath + ".tmp"
	data := marshalManagedCLIReleaseSidecar(metadata, ref, binaryPath)
	if err := os.WriteFile(tempPath, []byte(data), 0o644); err != nil {
		return errors.Wrap(err, "write managed cli release sidecar")
	}
	if err := os.Rename(tempPath, sidecarPath); err != nil {
		return errors.Wrap(err, "commit managed cli release sidecar")
	}
	return nil
}

func marshalManagedCLIReleaseSidecar(
	metadata *spacewave_release.ReleaseMetadata,
	ref *bldr_manifest.ManifestRef,
	binaryPath string,
) string {
	meta := ref.GetMeta()
	var data []byte
	appendStringField := func(key string, val string, trailing bool) {
		data = append(data, "  "...)
		data = strconv.AppendQuote(data, key)
		data = append(data, ": "...)
		data = strconv.AppendQuote(data, val)
		if trailing {
			data = append(data, ',')
		}
		data = append(data, '\n')
	}
	appendUintField := func(key string, val uint64, trailing bool) {
		data = append(data, "  "...)
		data = strconv.AppendQuote(data, key)
		data = append(data, ": "...)
		data = strconv.AppendUint(data, val, 10)
		if trailing {
			data = append(data, ',')
		}
		data = append(data, '\n')
	}

	data = append(data, "{\n"...)
	appendStringField("binary_path", binaryPath, true)
	appendStringField("project_id", metadata.GetProjectId(), true)
	appendStringField("entrypoint_role", "cli", true)
	appendStringField("channel_key", metadata.GetChannelKey(), true)
	appendStringField("manifest_id", meta.GetManifestId(), true)
	appendUintField("manifest_rev", meta.GetRev(), true)
	appendStringField("platform_id", meta.GetPlatformId(), true)
	appendStringField("manifest_ref", ref.GetManifestRef().MarshalString(), false)
	data = append(data, "}\n"...)
	return string(data)
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
