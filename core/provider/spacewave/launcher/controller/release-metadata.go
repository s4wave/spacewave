//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
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

// refreshCurrentReleaseMetadataStatus stages the latest accepted distribution configuration.
func (c *Controller) refreshCurrentReleaseMetadataStatus(ctx context.Context) error {
	// Wait for an accepted distribution before resolving its release.
	info, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	return c.refreshReleaseMetadataStatus(ctx, info.GetDistConfig())
}

// refreshReleaseMetadataStatus resolves release metadata for the current DistConfig.
func (c *Controller) refreshReleaseMetadataStatus(ctx context.Context, distConf *spacewave_launcher.DistConfig) error {
	// Withdraw staged status when no distribution is selected.
	if distConf.GetRev() == 0 {
		c.clearUpdateState()
		c.setSelectedEntrypointManifestRef(nil)
		c.setSelectedCLIManifestRef(nil, "")
		c.setReleaseMetadataOutcome("idle")
		_ = c.clearManagedCLIReleaseSidecar()
		return nil
	}

	// Clear prior selection before reading the new channel.
	c.setReleaseMetadataOutcome("resolving")
	c.setSelectedEntrypointManifestRef(nil)
	c.setSelectedCLIManifestRef(nil, "")
	c.setReleaseWorldHeadRef("")
	_ = c.clearManagedCLIReleaseSidecar()

	// Resolve the channel from the mounted release World.
	metadata, err := c.resolveReleaseMetadata(ctx, distConf.ResolvedChannelKey())
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}

	// Select only manifests compatible with this desktop and application.
	platformID, err := nativeDesktopPlatformID()
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	manifestRef, cliManifestRef, err := c.conf.SelectReleaseManifests(metadata, platformID)
	if err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}

	// Stage the selected artifacts before exposing a restart action.
	if err := c.stageReleaseManifestUpdate(ctx, metadata, platformID, manifestRef, cliManifestRef); err != nil {
		c.setUpdateError(err)
		c.setReleaseMetadataOutcome("error")
		return err
	}
	return nil
}

// resolveReleaseMetadata reads channel metadata and its matching World revision.
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

// stageReleaseManifestUpdate checks out and verifies the selected executable set.
func (c *Controller) stageReleaseManifestUpdate(
	ctx context.Context,
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
	manifestRef *bldr_manifest.ManifestRef,
	cliManifestRef *bldr_manifest.ManifestRef,
) error {
	// Require every executable promised by the application configuration.
	if manifestRef == nil {
		return errors.New("release metadata missing configured native entrypoint for platform " + platformID)
	}
	if cliManifestRef == nil && !c.conf.GetDisableCliUpdate() {
		return errors.New("release metadata missing configured CLI entrypoint for platform " + platformID)
	}

	// Publish selection and allocate the version-specific staging roots.
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

	// Keep desktop and companion contents in separate verified checkouts.
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

	// Read both artifacts within the same release World transaction.
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
		if cliManifestRef == nil {
			return nil
		}

		// Materialize the companion only for applications that distribute one.
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

	// Verify all selected executables before marking the update staged.
	if err := c.verifyStagedReleaseEntrypoint(ctx, platformID, stageRoot, stagedPath); err != nil {
		return err
	}
	if cliManifestRef != nil {
		if err := verifyStagedCLIEntrypoint(stageRoot, cliDistPath, cliStagedPath); err != nil {
			return err
		}
		if err := c.writeManagedCLIReleaseSidecar(stagingDir, metadata, cliManifestRef, cliStagedPath); err != nil {
			return err
		}
	}

	// Publish readiness only after artifact verification and CLI discovery.
	c.setSelectedCLIManifestRef(cliManifestRef, cliStagedPath)
	current, err := c.stagedReleaseIsCurrent(stagedPath)
	if err != nil {
		return err
	}
	if current {
		c.clearUpdateState()
		c.setReleaseMetadataOutcome("current")
		return nil
	}
	c.setUpdateStaged(metadata.GetVersion(), stagedPath)
	return nil
}

// resolveStagingDir returns the launcher-owned update staging directory.
func (c *Controller) resolveStagingDir() (string, error) {
	if c.stagingDirFunc != nil {
		return c.stagingDirFunc()
	}
	if projectID := c.conf.GetProjectId(); projectID != "" && projectID != "spacewave" {
		root, err := storagepath.DetermineStorageRoot(projectID)
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "updates"), nil
	}
	return getStagingDir()
}

// setUpdateDownloading publishes the version currently being staged.
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

// setUpdateStaged publishes a verified replacement ready for an explicit restart.
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

// setReleaseMetadataOutcome updates release-resolution diagnostics.
func (c *Controller) setReleaseMetadataOutcome(outcome string) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.ReleaseMetadataOutcome = outcome
	})
}

// setReleaseWorldHeadRef records the revision used to select the release.
func (c *Controller) setReleaseWorldHeadRef(ref string) {
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.ReleaseWorldHeadRef = ref
	})
}

// setSelectedEntrypointManifestRef replaces desktop selection diagnostics.
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

// setSelectedCLIManifestRef replaces companion CLI selection diagnostics.
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

// stagedManifestEntrypointPath resolves a slash-relative entrypoint within its checkout.
func stagedManifestEntrypointPath(distPath string, entrypoint string) (string, error) {
	clean := path.Clean(entrypoint)
	if clean == "." || clean == ".." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\\") {
		return "", errors.New("manifest entrypoint must be a local relative path")
	}

	// Check containment again using native filesystem semantics.
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

// releaseVersionStagingRoot confines a release version to one staging directory.
func releaseVersionStagingRoot(stagingDir string, version string) (string, error) {
	clean := path.Clean(version)
	if clean == "." || clean == ".." || clean != version || path.IsAbs(clean) ||
		strings.Contains(clean, "/") || strings.Contains(version, "\\") {
		return "", errors.New("release version must be a local path segment")
	}

	// Confine the resulting native path to the staging directory.
	stageRoot := filepath.Join(stagingDir, filepath.FromSlash(clean))
	if err := verifyPathInsideRoot(stagingDir, stageRoot, "release staging root"); err != nil {
		return "", err
	}
	return stageRoot, nil
}

// prepareReleaseStagingRoot creates staging directories without following existing symlinks.
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

	// Reject unsafe checkout roots before any manifest writes begin.
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

// verifyStagedCLIEntrypoint requires a regular executable inside its checkout.
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

	// Reject non-file entrypoints without following the final path component.
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

	// Grant executable permissions only after validating the complete path.
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return errors.Wrap(err, "chmod staged cli entrypoint")
	}
	return nil
}

// verifyPathInsideRoot rejects paths outside or equal to the containing root.
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

// rejectExistingNonDirectoryOrSymlink allows absent directories but rejects unsafe existing paths.
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

// requireDirectoryNotSymlink requires an existing directory with no final symlink.
func requireDirectoryNotSymlink(dir string, label string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return errors.Wrap(err, "stat "+label)
	}
	return validateDirectoryNotSymlink(info, label)
}

// validateDirectoryNotSymlink checks directory metadata without following links.
func validateDirectoryNotSymlink(info os.FileInfo, label string) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New(label + " must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New(label + " must be a directory")
	}
	return nil
}

// verifyNoSymlinkPath rejects symlinks in the entrypoint parent chain.
func verifyNoSymlinkPath(rootPath string, filePath string) error {
	rel, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("staged cli entrypoint escapes cli dist root")
	}

	// Inspect every parent component without resolving symlinks.
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

// clearManagedCLIReleaseSidecar withdraws the previously selected companion executable.
func (c *Controller) clearManagedCLIReleaseSidecar() error {
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return err
	}

	// An already absent discovery file is the desired withdrawn state.
	err = os.Remove(filepath.Join(stagingDir, managedCLIReleaseSidecarFilename))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

// writeManagedCLIReleaseSidecar publishes the verified companion executable location.
func (c *Controller) writeManagedCLIReleaseSidecar(
	stagingDir string,
	metadata *spacewave_release.ReleaseMetadata,
	ref *bldr_manifest.ManifestRef,
	binaryPath string,
) error {
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return errors.Wrap(err, "create managed cli release sidecar dir")
	}

	// Replace discovery only after writing the complete new record.
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

// marshalManagedCLIReleaseSidecar encodes the CLI discovery record with JSON escaping.
func marshalManagedCLIReleaseSidecar(
	metadata *spacewave_release.ReleaseMetadata,
	ref *bldr_manifest.ManifestRef,
	binaryPath string,
) string {
	meta := ref.GetMeta()
	var arena fastjson.Arena
	marshalString := func(value string) string {
		return string(arena.NewString(value).MarshalTo(nil))
	}

	// Escape string values while retaining the existing discovery schema.
	var b strings.Builder
	b.WriteString("{\n")
	b.WriteString("  \"binary_path\": ")
	b.WriteString(marshalString(binaryPath))
	b.WriteString(",\n  \"project_id\": ")
	b.WriteString(marshalString(metadata.GetProjectId()))
	b.WriteString(",\n  \"entrypoint_role\": ")
	b.WriteString(marshalString("cli"))
	b.WriteString(",\n  \"channel_key\": ")
	b.WriteString(marshalString(metadata.GetChannelKey()))
	b.WriteString(",\n  \"manifest_id\": ")
	b.WriteString(marshalString(meta.GetManifestId()))
	b.WriteString(",\n  \"manifest_rev\": ")
	b.WriteString(strconv.FormatUint(meta.GetRev(), 10))
	b.WriteString(",\n  \"platform_id\": ")
	b.WriteString(marshalString(meta.GetPlatformId()))
	b.WriteString(",\n  \"manifest_ref\": ")
	b.WriteString(marshalString(ref.GetManifestRef().MarshalString()))
	b.WriteString("\n}\n")
	return b.String()
}

// verifyStagedReleaseEntrypoint enforces the installed platform executable shape.
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

	// Installed macOS bundles must remain signed bundles after an update.
	if isDarwinDesktopPlatform(platformID) {
		if err := c.verifyDarwinInstalledAppStagedEntrypoint(stageRoot, stagedPath, stagedInfo.IsDir()); err != nil {
			return err
		}
	}
	if !stagedInfo.IsDir() {
		return nil
	}

	// Directory entrypoints are accepted only as verified app bundles.
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

// verifyDarwinInstalledAppStagedEntrypoint prevents replacing an installed app bundle with a raw binary.
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

// readReleaseMetadataSnapshot reads metadata and its root reference in one World snapshot.
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

// readSelectedReleaseMetadata resolves and validates the requested release channel.
func readSelectedReleaseMetadata(
	ctx context.Context,
	ws world.WorldState,
	channelKey string,
) (*spacewave_release.ReleaseMetadata, error) {
	if channelKey == "" {
		return nil, errors.New("release channel key is empty")
	}

	// Validate the channel directory before resolving its selected release.
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

	// Require an explicit directory entry for the requested channel.
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

	// Validate the selected record and its channel identity.
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

// readReleaseMetadataBlock decodes a typed release object within the caller's read scope.
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

	// Decode through the existing object read scope.
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

// releaseMetadataObjectKey locates one channel in the release metadata directory.
func releaseMetadataObjectKey(channelKey string) string {
	return path.Join(releaseMetadataDirectoryObjectKey, channelKey)
}

// releaseMetadataSupportsPlatform reports whether the default desktop manifest exists.
func releaseMetadataSupportsPlatform(metadata *spacewave_release.ReleaseMetadata, platformID string) bool {
	_, err := selectReleaseManifestRef(metadata, platformID)
	return err == nil
}

// selectReleaseManifestRef selects the default Spacewave desktop entrypoint.
func selectReleaseManifestRef(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
) (*bldr_manifest.ManifestRef, error) {
	return selectReleaseManifestRefByID(metadata, platformID, nativeEntrypointManifestID, "native")
}

// selectCLIReleaseManifestRef selects the default Spacewave CLI entrypoint.
func selectCLIReleaseManifestRef(
	metadata *spacewave_release.ReleaseMetadata,
	platformID string,
) (*bldr_manifest.ManifestRef, error) {
	return selectReleaseManifestRefByID(metadata, platformID, cliEntrypointManifestID, "cli")
}

// selectReleaseManifestRefByID requires exactly one matching manifest for a platform.
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

		// Reject ambiguous entrypoints instead of choosing by manifest order.
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

	// Return the unique match or explain which entrypoint is missing.
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

// checkoutReleaseManifest materializes manifest contents through UnixFS.
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

// nativeDesktopPlatformID returns the canonical platform of this desktop process.
func nativeDesktopPlatformID() (string, error) {
	platform, err := bldr_platform.ParseNativePlatform("desktop")
	if err != nil {
		return "", err
	}
	return platform.GetPlatformID(), nil
}

// isDarwinDesktopPlatform recognizes macOS desktop platform identifiers.
func isDarwinDesktopPlatform(platformID string) bool {
	return strings.HasPrefix(platformID, "desktop/darwin/")
}
