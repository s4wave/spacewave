//go:build !js

package bldr_manifest_builder

import (
	"context"
	"path/filepath"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

type manifestCommitIdentityContextKey struct{}

type manifestCommitTimestampContextKey struct{}

type manifestCommitIdentity struct {
	existingBuilderResult *BuilderResult
	manifestDeps          []*InputManifest_ManifestDep
}

// Validate validates the configuration.
func (c *BuilderConfig) Validate() error {
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if err := c.GetManifestMeta().Validate(false); err != nil {
		return err
	}
	if err := c.GetBuildPolicy().Validate(); err != nil {
		return err
	}
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if c.GetSourcePath() == "" {
		return errors.Wrap(manifest.ErrEmptyPath, "source path")
	}
	if !filepath.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if c.GetWorkingPath() == "" {
		return errors.Wrap(manifest.ErrEmptyPath, "working path")
	}
	if !filepath.IsAbs(c.GetWorkingPath()) {
		return errors.New("working path must be absolute")
	}
	return nil
}

// WithManifestCommitIdentity attaches the previous manifest and current watched
// manifest refs used to decide whether a commit publishes a new revision.
func WithManifestCommitIdentity(
	ctx context.Context,
	existingBuilderResult *BuilderResult,
	manifestDeps []*InputManifest_ManifestDep,
) context.Context {
	if existingBuilderResult == nil && len(manifestDeps) == 0 {
		return ctx
	}
	var existingBuilderResultClone *BuilderResult
	if existingBuilderResult != nil {
		existingBuilderResultClone = existingBuilderResult.CloneVT()
	}
	return context.WithValue(ctx, manifestCommitIdentityContextKey{}, &manifestCommitIdentity{
		existingBuilderResult: existingBuilderResultClone,
		manifestDeps:          cloneManifestDeps(manifestDeps),
	})
}

func withManifestCommitTimestamp(ctx context.Context, ts *timestamp.Timestamp) context.Context {
	if ts == nil {
		return ctx
	}
	return context.WithValue(ctx, manifestCommitTimestampContextKey{}, ts.CloneVT())
}

// ParsePeerID parses the peer id field.
func (c *BuilderConfig) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// CommitManifest is a shortcut for CommitManifest.
func (c *BuilderConfig) CommitManifest(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFs,
	assetsFs billy.Filesystem,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	pid, err := c.ParsePeerID()
	if err != nil {
		return nil, nil, err
	}
	ts := manifestCommitTimestamp(ctx)

	var manifestValue *manifest.Manifest
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		var err error
		manifestValue, err = manifest.CreateManifestWithBilly(
			ctx,
			bcs,
			meta,
			entrypointFilename,
			distFs,
			assetsFs,
			ts,
		)
		return err
	})
	if err != nil {
		return nil, manifestRef, err
	}

	if existingManifest, existingRef := reuseManifestCommit(ctx, manifestValue); existingManifest != nil {
		return existingManifest, existingRef, nil
	}

	manifestValue.GetMeta().Logger(le).
		WithField("object-key", c.GetObjectKey()).
		WithField("link-object-keys", c.GetLinkObjectKeys()).
		Info("committing manifest to world")
	_, _, err = ws.ApplyWorldOp(
		ctx,
		manifest_world.NewStoreManifestOp(
			c.GetObjectKey(),
			c.GetLinkObjectKeys(),
			manifest.NewManifestRef(
				manifestValue.GetMeta(),
				manifestRef,
			),
		),
		pid,
	)
	if err != nil {
		return nil, nil, err
	}
	return manifestValue, manifestRef, nil
}

// CommitManifestWithPaths is a shortcut for CommitManifest with on-disk paths.
func (c *BuilderConfig) CommitManifestWithPaths(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	meta *manifest.ManifestMeta,
	entrypointFilename string,
	distFsPath,
	assetsFsPath string,
) (*manifest.Manifest, *bucket.ObjectRef, error) {
	var distFs billy.Filesystem
	if distFsPath != "" {
		distFs = osfs.New(distFsPath, osfs.WithBoundOS())
	}

	var assetsFs billy.Filesystem
	if assetsFsPath != "" {
		assetsFs = osfs.New(assetsFsPath, osfs.WithBoundOS())
	}

	return c.CommitManifest(ctx, le, ws, meta, entrypointFilename, distFs, assetsFs)
}

// CheckoutManifest is a shortcut for CheckoutManifest.
//
// If either of the paths are empty, they will be skipped.
// If manifestRef is nil, will use the reference defaulted to by accessFunc.
func (c *BuilderConfig) CheckoutManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessFunc world.AccessWorldStateFunc,
	manifestRef *bucket.ObjectRef,
	distFsPath,
	assetsFsPath string,
) (*manifest.Manifest, error) {
	return manifest_world.CheckoutManifest(
		ctx,
		le,
		accessFunc,
		manifestRef,
		distFsPath,
		assetsFsPath,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		nil,
		nil,
	)
}

func manifestCommitTimestamp(ctx context.Context) *timestamp.Timestamp {
	ts, ok := ctx.Value(manifestCommitTimestampContextKey{}).(*timestamp.Timestamp)
	if ok && ts != nil {
		return ts.CloneVT()
	}
	return timestamp.Now()
}

func reuseManifestCommit(
	ctx context.Context,
	manifestValue *manifest.Manifest,
) (*manifest.Manifest, *bucket.ObjectRef) {
	identity, ok := ctx.Value(manifestCommitIdentityContextKey{}).(*manifestCommitIdentity)
	if !ok || identity == nil {
		return nil, nil
	}
	existingResult := identity.existingBuilderResult
	if existingResult == nil {
		return nil, nil
	}
	existingManifest := existingResult.GetManifest()
	if !manifestCommitOutputsEqual(existingManifest, manifestValue) {
		return nil, nil
	}
	if !manifestDepsEqual(existingResult.GetInputManifest().GetManifestDeps(), identity.manifestDeps) {
		return nil, nil
	}
	return existingManifest.CloneVT(), existingResult.GetManifestRef().GetManifestRef().Clone()
}

func manifestCommitOutputsEqual(existingManifest, manifestValue *manifest.Manifest) bool {
	if existingManifest == nil || manifestValue == nil {
		return false
	}
	existingMeta := existingManifest.GetMeta()
	meta := manifestValue.GetMeta()
	if existingMeta.GetManifestId() != meta.GetManifestId() {
		return false
	}
	if existingMeta.GetBuildType() != meta.GetBuildType() {
		return false
	}
	if existingMeta.GetPlatformId() != meta.GetPlatformId() {
		return false
	}
	if existingMeta.GetDescription() != meta.GetDescription() {
		return false
	}
	if existingManifest.GetEntrypoint() != manifestValue.GetEntrypoint() {
		return false
	}
	if !existingManifest.GetDistFsRef().EqualVT(manifestValue.GetDistFsRef()) {
		return false
	}
	return existingManifest.GetAssetsFsRef().EqualVT(manifestValue.GetAssetsFsRef())
}

func manifestDepsEqual(
	cachedDeps []*InputManifest_ManifestDep,
	currentDeps []*InputManifest_ManifestDep,
) bool {
	if len(cachedDeps) != len(currentDeps) {
		return false
	}
	cachedByID := make(map[string]*InputManifest_ManifestDep, len(cachedDeps))
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

func cloneManifestDeps(deps []*InputManifest_ManifestDep) []*InputManifest_ManifestDep {
	if len(deps) == 0 {
		return nil
	}
	cloned := make([]*InputManifest_ManifestDep, len(deps))
	for i, dep := range deps {
		if dep != nil {
			cloned[i] = dep.CloneVT()
		}
	}
	return cloned
}
