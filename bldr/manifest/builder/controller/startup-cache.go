//go:build !js

package bldr_manifest_builder_controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// startupCacheFormatEnvKey is bumped when compiler-owned output policy changes
// without changing a plugin source file. V9 requires persisted nested builder
// provenance and valid asset-only Manifest entrypoints.
const startupCacheFormatEnvKey = "BLDR_STARTUP_CACHE_FORMAT_V9"

// startupValidationResult contains the startup cache validation result.
type startupValidationResult struct {
	// builderResult is the validated startup builder result.
	builderResult *bldr_manifest_builder.BuilderResult
	// manifestDepSnapshot holds the current manifest dependency refs.
	manifestDepSnapshot map[string]*bucket.ObjectRef
	// subManifestIDs contains recursively validated child Manifest IDs.
	subManifestIDs []string
	// reason describes why startup reuse was rejected.
	reason string
}

// validateStartupBuilderResult validates the configured startup builder result.
func (c *Controller) validateStartupBuilderResult(
	ctx context.Context,
	le *logrus.Entry,
	builderCtrl bldr_manifest_builder.Controller,
) (*startupValidationResult, error) {
	startupBuilderResult := c.c.GetStartupBuilderResult()
	if startupBuilderResult == nil {
		return &startupValidationResult{reason: "no startup builder result"}, nil
	}
	if !builderCtrl.SupportsStartupManifestCache() {
		return &startupValidationResult{reason: "builder is not startup-cache-safe"}, nil
	}
	if err := startupBuilderResult.Validate(); err != nil {
		return &startupValidationResult{
			reason: errors.Wrap(err, "invalid startup builder result").Error(),
		}, nil
	}

	inputManifest := startupBuilderResult.GetInputManifest()
	if inputManifest == nil {
		return &startupValidationResult{reason: "startup builder result has no input manifest"}, nil
	}

	if err := validateStartupFiles(c.c.GetBuilderConfig().GetSourcePath(), inputManifest); err != nil {
		return &startupValidationResult{reason: err.Error()}, nil
	}
	if err := validateStartupInputs(c.c.GetControllerConfig(), inputManifest); err != nil {
		return &startupValidationResult{reason: err.Error()}, nil
	}
	reason, err := c.validateStartupManifestAvailability(ctx, le, startupBuilderResult)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return &startupValidationResult{reason: reason}, nil
	}

	subManifestIDs, reason, err := c.validateStartupSubManifestResults(ctx, le, startupBuilderResult)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return &startupValidationResult{reason: reason}, nil
	}

	manifestDepSnapshot, err := c.validateStartupManifestDeps(ctx, le, inputManifest)
	if err != nil {
		return nil, err
	}
	if manifestDepSnapshot == nil && len(inputManifest.GetManifestDeps()) != 0 {
		return &startupValidationResult{reason: "manifest dependency configuration changed"}, nil
	}

	return &startupValidationResult{
		builderResult:       startupBuilderResult.CloneVT(),
		manifestDepSnapshot: manifestDepSnapshot,
		subManifestIDs:      subManifestIDs,
	}, nil
}

// validateStartupManifestAvailability verifies the cached manifest DAG is
// readable in the current world storage.
func (c *Controller) validateStartupManifestAvailability(
	ctx context.Context,
	le *logrus.Entry,
	startupBuilderResult *bldr_manifest_builder.BuilderResult,
) (string, error) {
	builderConfig := c.c.GetBuilderConfig()
	engineID := builderConfig.GetEngineId()
	if engineID == "" {
		return "", nil
	}

	manifestRef := startupBuilderResult.GetManifestRef()
	if manifestRef == nil || manifestRef.GetManifestRef() == nil {
		return "startup builder result has no manifest ref", nil
	}

	ws := world.NewEngineWorldState(world.NewBusEngine(ctx, c.bus, engineID), false)
	cachedBucketID := manifestRef.GetManifestRef().GetBucketId()
	var currentVolumeID string
	if builderConfig.GetPeerId() != "" {
		peerID, err := peer.IDB58Decode(builderConfig.GetPeerId())
		if err != nil {
			return "", err
		}
		currentVolumeID = volume.NewVolumeID(volume_bolt.ControllerID, peerID)
	}
	var currentBucketID string
	if currentVolumeID == "" {
		storageCursor, err := ws.BuildStorageCursor(ctx)
		if err != nil {
			return "", err
		}
		defer storageCursor.Release()
		currentVolumeID = storageCursor.GetOpArgs().GetVolumeId()
		currentBucketID = storageCursor.GetRefWithOpArgs().GetBucketId()
	}
	if currentVolumeID != "" && cachedBucketID != "" {
		le.WithFields(logrus.Fields{
			"bucket-id": cachedBucketID,
			"volume-id": currentVolumeID,
		}).Debug("validating startup manifest bucket availability")
		bh, _, bhRef, err := bucket.ExBuildBucketAPI(ctx, c.bus, true, cachedBucketID, currentVolumeID, nil)
		if err != nil {
			return "", err
		}
		if bhRef != nil {
			defer bhRef.Release()
		}
		if bh == nil || !bh.GetExists() {
			return errors.Errorf(
				"startup manifest bucket %q is not in current world volume %q",
				cachedBucketID,
				currentVolumeID,
			).Error(), nil
		}
	}
	if currentBucketID != "" && cachedBucketID != "" && currentBucketID != cachedBucketID {
		return errors.Errorf(
			"startup manifest bucket changed: %q != %q",
			cachedBucketID,
			currentBucketID,
		).Error(), nil
	}

	entrypoint := startupBuilderResult.GetManifest().GetEntrypoint()
	err := bldr_manifest_world.AccessStartupManifest(
		ctx,
		le,
		ws.AccessWorldState,
		manifestRef.GetManifestRef(),
		func(
			ctx context.Context,
			_ *bucket_lookup.Cursor,
			_ *block.Cursor,
			_ *bldr_manifest.Manifest,
			distFS,
			_ *unixfs.FSHandle,
		) error {
			if entrypoint == "" {
				return nil
			}
			entrypointHandle, _, err := distFS.LookupPath(ctx, entrypoint)
			if err != nil {
				return errors.Wrap(err, "lookup startup entrypoint")
			}
			defer entrypointHandle.Release()
			if _, err := entrypointHandle.GetFileInfo(ctx); err != nil {
				return errors.Wrap(err, "stat startup entrypoint")
			}
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "access startup manifest").Error(), nil
	}
	return "", nil
}

func (c *Controller) validateStartupSubManifestResults(
	ctx context.Context,
	le *logrus.Entry,
	builderResult *bldr_manifest_builder.BuilderResult,
) ([]string, string, error) {
	var manifestIDs []string
	for _, subManifestID := range slices.Sorted(maps.Keys(builderResult.GetSubManifestResults())) {
		subManifestResult := builderResult.GetSubManifestResults()[subManifestID]
		if err := subManifestResult.Validate(); err != nil {
			return nil, errors.Wrapf(err, "invalid startup sub-manifest %q", subManifestID).Error(), nil
		}
		inputManifest := subManifestResult.GetInputManifest()
		// The parent InputManifest contains the transitive child file union.
		// Rechecking files here would hash every nested input twice.
		if err := validateNestedStartupInputs(inputManifest); err != nil {
			return nil, errors.Wrapf(err, "validate startup sub-manifest %q", subManifestID).Error(), nil
		}
		reason, err := c.validateStartupManifestAvailability(ctx, le, subManifestResult)
		if err != nil {
			return nil, "", err
		}
		if reason != "" {
			return nil, errors.Wrapf(errors.New(reason), "validate startup sub-manifest %q", subManifestID).Error(), nil
		}
		nestedManifestIDs, reason, err := c.validateStartupSubManifestResults(ctx, le, subManifestResult)
		if err != nil || reason != "" {
			return nil, reason, err
		}
		manifestIDs = append(manifestIDs, nestedManifestIDs...)
		manifestIDs = append(manifestIDs, subManifestResult.GetManifest().GetMeta().GetManifestId())
	}
	return manifestIDs, "", nil
}

// validateStartupManifestDeps validates the cached manifest dependency refs.
func (c *Controller) validateStartupManifestDeps(
	ctx context.Context,
	le *logrus.Entry,
	inputManifest *bldr_manifest_builder.InputManifest,
) (map[string]*bucket.ObjectRef, error) {
	watchManifestIDs := c.c.GetWatchManifestIds()
	cachedDeps := inputManifest.GetManifestDeps()
	if len(watchManifestIDs) == 0 {
		if len(cachedDeps) != 0 {
			return nil, nil
		}
		return map[string]*bucket.ObjectRef{}, nil
	}

	resolvedDeps, refs := c.resolveManifestDeps(ctx, le, watchManifestIDs)
	if !manifestDepsEqual(cachedDeps, resolvedDeps) {
		return nil, nil
	}
	return refs, nil
}

// enrichBuilderResultForStartupReuse adds generic startup validation inputs.
func enrichBuilderResultForStartupReuse(
	builderConfig *bldr_manifest_builder.BuilderConfig,
	controllerConfig *configset_proto.ControllerConfig,
	builderResult *bldr_manifest_builder.BuilderResult,
) error {
	if builderResult == nil {
		return nil
	}
	inputManifest := builderResult.GetInputManifest()
	if inputManifest == nil {
		return nil
	}

	if err := captureFileIdentities(builderConfig.GetSourcePath(), inputManifest); err != nil {
		return err
	}

	controllerConfigDigest, err := marshalControllerConfigDigest(controllerConfig)
	if err != nil {
		return err
	}
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	inputManifest.AddStartupInput(newStartupCacheFormatInput())
	inputManifest.SortStartupInputs()
	inputManifest.SortFiles()
	return nil
}

func addSubManifestResultForStartupReuse(
	builderResult *bldr_manifest_builder.BuilderResult,
	subManifestID string,
	subManifestResult *bldr_manifest_builder.BuilderResult,
) error {
	if builderResult == nil || subManifestResult == nil {
		return nil
	}
	if err := subManifestResult.Validate(); err != nil {
		return errors.Wrapf(err, "invalid sub-manifest result %q", subManifestID)
	}
	inputManifest := builderResult.GetInputManifest()
	if inputManifest == nil {
		return errors.New("builder result has no input manifest")
	}

	if builderResult.SubManifestResults == nil {
		builderResult.SubManifestResults = make(map[string]*bldr_manifest_builder.BuilderResult)
	}
	builderResult.SubManifestResults[subManifestID] = subManifestResult.CloneVT()

	seenPaths := make(map[string]struct{}, len(inputManifest.GetFiles()))
	for _, inputFile := range inputManifest.GetFiles() {
		seenPaths[path.Clean(inputFile.GetPath())] = struct{}{}
	}
	for _, inputFile := range subManifestResult.GetInputManifest().GetFiles() {
		cleanPath := path.Clean(inputFile.GetPath())
		if _, ok := seenPaths[cleanPath]; ok {
			continue
		}
		startupFile := inputFile.CloneVT()
		startupFile.StartupOnly = true
		inputManifest.Files = append(inputManifest.Files, startupFile)
		seenPaths[cleanPath] = struct{}{}
	}
	inputManifest.SortFiles()
	return nil
}

// validateStartupFiles validates cached file identities against the filesystem.
func validateStartupFiles(sourcePath string, inputManifest *bldr_manifest_builder.InputManifest) error {
	for _, inputFile := range inputManifest.GetFiles() {
		fileIdentity := inputFile.GetIdentity()
		if fileIdentity == nil {
			return errors.Errorf("startup file %q is missing cached identity", inputFile.GetPath())
		}
		filePath := resolveStartupInputPath(sourcePath, inputFile.GetPath())
		match, err := fileIdentity.MatchesFile(filePath)
		if err != nil {
			return errors.Wrapf(err, "validate startup file %q", inputFile.GetPath())
		}
		if !match {
			return errors.Errorf("startup file %q changed", inputFile.GetPath())
		}
	}
	return nil
}

// validateStartupInputs validates typed non-file startup inputs.
func validateStartupInputs(
	controllerConfig *configset_proto.ControllerConfig,
	inputManifest *bldr_manifest_builder.InputManifest,
) error {
	return validateStartupInputsWithControllerConfig(controllerConfig, inputManifest, true)
}

// validateNestedStartupInputs validates child inputs whose effective config is
// derived from and covered by the validated parent controller config.
func validateNestedStartupInputs(inputManifest *bldr_manifest_builder.InputManifest) error {
	return validateStartupInputsWithControllerConfig(nil, inputManifest, false)
}

func validateStartupInputsWithControllerConfig(
	controllerConfig *configset_proto.ControllerConfig,
	inputManifest *bldr_manifest_builder.InputManifest,
	validateControllerConfig bool,
) error {
	var controllerConfigDigest []byte
	var foundControllerConfigDigest bool
	var foundStartupCacheFormat bool
	for _, input := range inputManifest.GetStartupInputs() {
		switch input.GetKind() {
		case bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR:
			if input.GetKey() == startupCacheFormatEnvKey {
				foundStartupCacheFormat = true
			}
			if os.Getenv(input.GetKey()) != input.GetStringValue() {
				return errors.Errorf("startup env %q changed", input.GetKey())
			}
		case bldr_manifest_builder.InputManifest_StartupInputKind_CONTROLLER_CONFIG_DIGEST:
			foundControllerConfigDigest = true
			if !validateControllerConfig {
				continue
			}
			if len(controllerConfigDigest) == 0 {
				digest, err := marshalControllerConfigDigest(controllerConfig)
				if err != nil {
					return err
				}
				controllerConfigDigest = digest
			}
			if !bytes.Equal(controllerConfigDigest, input.GetBytesValue()) {
				return errors.New("builder controller config changed")
			}
		default:
			return errors.Errorf("unsupported startup input kind: %s", input.GetKind().String())
		}
	}
	if !foundControllerConfigDigest {
		return errors.New("missing builder controller config digest")
	}
	if !foundStartupCacheFormat {
		return errors.New("missing startup cache format marker")
	}
	return nil
}

func newStartupCacheFormatInput() *bldr_manifest_builder.InputManifest_StartupInput {
	return bldr_manifest_builder.NewEnvStartupInput(
		startupCacheFormatEnvKey,
		os.Getenv(startupCacheFormatEnvKey),
	)
}

// captureFileIdentities captures file identities on all input manifest files.
func captureFileIdentities(sourcePath string, inputManifest *bldr_manifest_builder.InputManifest) error {
	for _, inputFile := range inputManifest.GetFiles() {
		fileIdentity, err := bldr_manifest_builder.CaptureFileIdentity(
			resolveStartupInputPath(sourcePath, inputFile.GetPath()),
		)
		if err != nil {
			return errors.Wrapf(err, "capture startup identity for %q", inputFile.GetPath())
		}
		inputFile.Identity = fileIdentity
	}
	return nil
}

// resolveStartupInputPath resolves manifest file paths under the source tree.
// Some bundlers emit relative paths that unnecessarily escape the source root
// before re-entering it (for example "../../node_modules/..."). For startup
// validation we resolve those back to the matching file under sourcePath.
func resolveStartupInputPath(sourcePath, inputPath string) string {
	cleanPath := filepath.Clean(inputPath)
	filePath := filepath.Join(sourcePath, cleanPath)
	if !strings.HasPrefix(cleanPath, "..") {
		return filePath
	}

	pathParts := strings.FieldsFunc(cleanPath, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for i, part := range pathParts {
		if part == ".." {
			continue
		}
		candidate := filepath.Join(pathParts[i:]...)
		for _, basePath := range []string{sourcePath, filepath.Join(sourcePath, ".bldr")} {
			candidatePath := filepath.Join(basePath, candidate)
			fileInfo, err := os.Stat(candidatePath)
			if err != nil || fileInfo.IsDir() {
				continue
			}
			return candidatePath
		}
	}

	return filePath
}

// marshalControllerConfigDigest marshals the controller config to a digest.
func marshalControllerConfigDigest(controllerConfig *configset_proto.ControllerConfig) ([]byte, error) {
	if controllerConfig == nil {
		return nil, nil
	}
	controllerConfigBin, err := controllerConfig.MarshalVT()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(controllerConfigBin)
	return digest[:], nil
}

// manifestDepsEqual compares cached and current manifest dependency snapshots.
func manifestDepsEqual(
	cachedDeps []*bldr_manifest_builder.InputManifest_ManifestDep,
	currentDeps []*bldr_manifest_builder.InputManifest_ManifestDep,
) bool {
	if len(cachedDeps) != len(currentDeps) {
		return false
	}
	cachedByID := make(map[string]*bldr_manifest_builder.InputManifest_ManifestDep, len(cachedDeps))
	for _, dep := range cachedDeps {
		cachedByID[dep.GetManifestId()] = dep
	}
	for _, dep := range currentDeps {
		cachedDep, ok := cachedByID[dep.GetManifestId()]
		if !ok {
			return false
		}
		if !cachedDep.GetManifestRef().EqualVT(dep.GetManifestRef()) {
			return false
		}
	}
	return true
}
