//go:build !js

package bldr_manifest_builder_controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// startupValidationResult contains the startup cache validation result.
type startupValidationResult struct {
	// builderResult is the validated startup builder result.
	builderResult *bldr_manifest_builder.BuilderResult
	// manifestDepSnapshot holds the current manifest dependency refs.
	manifestDepSnapshot map[string]*bucket.ObjectRef
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
	entrypoint := startupBuilderResult.GetManifest().GetEntrypoint()
	err := bldr_manifest_world.AccessManifest(
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
	inputManifest.SortStartupInputs()
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
		filePath := filepath.Join(sourcePath, inputFile.GetPath())
		currentIdentity, err := captureFileIdentity(filePath)
		if err != nil {
			return errors.Wrapf(err, "validate startup file %q", inputFile.GetPath())
		}
		if fileIdentity.GetSizeBytes() == currentIdentity.GetSizeBytes() &&
			fileIdentity.GetModTimeUnixNano() == currentIdentity.GetModTimeUnixNano() {
			continue
		}
		if bytes.Equal(fileIdentity.GetSha256(), currentIdentity.GetSha256()) {
			continue
		}
		return errors.Errorf("startup file %q changed", inputFile.GetPath())
	}
	return nil
}

// validateStartupInputs validates typed non-file startup inputs.
func validateStartupInputs(
	controllerConfig *configset_proto.ControllerConfig,
	inputManifest *bldr_manifest_builder.InputManifest,
) error {
	var controllerConfigDigest []byte
	var foundControllerConfigDigest bool
	for _, input := range inputManifest.GetStartupInputs() {
		switch input.GetKind() {
		case bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR:
			if os.Getenv(input.GetKey()) != input.GetStringValue() {
				return errors.Errorf("startup env %q changed", input.GetKey())
			}
		case bldr_manifest_builder.InputManifest_StartupInputKind_CONTROLLER_CONFIG_DIGEST:
			foundControllerConfigDigest = true
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
	return nil
}

// captureFileIdentities captures file identities on all input manifest files.
func captureFileIdentities(sourcePath string, inputManifest *bldr_manifest_builder.InputManifest) error {
	for _, inputFile := range inputManifest.GetFiles() {
		fileIdentity, err := captureFileIdentity(filepath.Join(sourcePath, inputFile.GetPath()))
		if err != nil {
			return errors.Wrapf(err, "capture startup identity for %q", inputFile.GetPath())
		}
		inputFile.Identity = fileIdentity
	}
	return nil
}

// captureFileIdentity captures the file identity for one path.
func captureFileIdentity(filePath string) (*bldr_manifest_builder.InputManifest_FileIdentity, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.IsDir() {
		return nil, errors.Errorf("path is a directory: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return nil, err
	}
	if fileInfo.Size() < 0 {
		return nil, errors.Errorf("negative file size: %d", fileInfo.Size())
	}
	return &bldr_manifest_builder.InputManifest_FileIdentity{
		// #nosec G115 -- fileInfo.Size() is validated as non-negative immediately above.
		SizeBytes:       uint64(fileInfo.Size()),
		ModTimeUnixNano: fileInfo.ModTime().UnixNano(),
		Sha256:          h.Sum(nil),
	}, nil
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
