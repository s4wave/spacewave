//go:build !js

package handoff

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

const cliEntrypointManifestID = "spacewave-cli"

var cliHandoffPlatformIDs = []string{
	"desktop/darwin/arm64",
	"desktop/darwin/amd64",
	"desktop/linux/arm64",
	"desktop/linux/amd64",
	"desktop/windows/arm64",
	"desktop/windows/amd64",
}

// CLIHandoffOptions carries inputs for writing a CLI release handoff.
type CLIHandoffOptions struct {
	RootDir             string
	CLIArtifactsDir     string
	ManifestPackDirsCSV string
	CLIRev              string
	GitSHA              string
	Tag                 string
	ReleaseEnvironment  string
	RunID               string
	RunAttempt          string
	SourceRepo          string
	Workflow            string
}

type cliHandoffManifest struct {
	manifestRefs []*cliHandoffManifestRef
	world        *entrypointHandoffEntry
	artifacts    []*entrypointHandoffEntry
	opts         CLIHandoffOptions
}

type cliHandoffManifestRef struct {
	ManifestID string
	PlatformID string
	Rev        uint64
	Ref        string
}

// WriteCLIHandoffManifest writes a cli-handoff.v1 root from imported Manifest packs.
func WriteCLIHandoffManifest(
	ctx context.Context,
	le *logrus.Entry,
	repoDir string,
	opts CLIHandoffOptions,
) error {
	// Require the CLI artifact paths, release identity, and source provenance.
	if opts.RootDir == "" {
		return errors.New("handoff root dir is required")
	}
	if opts.CLIArtifactsDir == "" {
		return errors.New("cli artifacts dir is required")
	}
	if opts.ManifestPackDirsCSV == "" {
		return errors.New("cli handoff manifest packs are required")
	}
	if opts.CLIRev == "" {
		return errors.New("cli rev is required")
	}
	if opts.ReleaseEnvironment == "" {
		return errors.New("release environment is required")
	}
	if opts.GitSHA == "" || opts.RunID == "" || opts.RunAttempt == "" || opts.SourceRepo == "" || opts.Workflow == "" {
		return errors.New("source provenance is required")
	}

	// Resolve the manifest packs that supply the CLI release.
	manifestPackDirs, err := expandManifestPackDirs(splitCSV(opts.ManifestPackDirsCSV))
	if err != nil {
		return err
	}
	if len(manifestPackDirs) == 0 {
		return errors.New("no cli manifest-pack artifacts found")
	}

	// Recreate the CLI handoff root for the staged release.
	if err := os.RemoveAll(opts.RootDir); err != nil {
		return errors.Wrap(err, "clean cli handoff root")
	}
	if err := os.MkdirAll(opts.RootDir, 0o755); err != nil {
		return errors.Wrap(err, "create cli handoff root")
	}

	// Import the CLI manifests and collect their platform references.
	manifestRefs, err := importAndCollectCLIHandoffManifestRefs(ctx, le, repoDir, manifestPackDirs)
	if err != nil {
		return err
	}

	// Stage the World containing the imported CLI manifests.
	worldEntry, err := stageCLIHandoffWorld(repoDir, opts.RootDir)
	if err != nil {
		return err
	}

	// Stage the native CLI archives under the handoff root.
	artifacts, err := stageCLIHandoffArtifacts(opts.RootDir, opts.CLIArtifactsDir)
	if err != nil {
		return err
	}

	// Write the CLI handoff manifest with its staged files and provenance.
	manifest := &cliHandoffManifest{
		manifestRefs: manifestRefs,
		world:        worldEntry,
		artifacts:    artifacts,
		opts:         opts,
	}
	if err := os.WriteFile(filepath.Join(opts.RootDir, "manifest.json"), []byte(marshalCLIHandoffManifest(manifest)), 0o644); err != nil {
		return errors.Wrap(err, "write cli handoff manifest")
	}
	return nil
}

func importAndCollectCLIHandoffManifestRefs(
	ctx context.Context,
	le *logrus.Entry,
	repoDir string,
	manifestPackDirs []string,
) ([]*cliHandoffManifestRef, error) {
	// Start the devtool bus for importing the CLI manifests.
	busHandle, err := startDevtoolBus(ctx, le, repoDir, false)
	if err != nil {
		return nil, err
	}
	defer busHandle.Release()

	// Import the CLI manifest packs into the devtool World.
	if err := importManifestPacksIntoBus(ctx, busHandle, manifestPackDirs); err != nil {
		return nil, err
	}

	// Collect the release references from the imported devtool World.
	refs, err := collectCLIHandoffManifestRefs(ctx, busHandle.GetWorldState())
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func collectCLIHandoffManifestRefs(
	ctx context.Context,
	ws world.WorldState,
) ([]*cliHandoffManifestRef, error) {
	// Collect the CLI manifests for every supported release platform.
	collected, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		cliEntrypointManifestID,
		cliHandoffPlatformIDs,
		"devtool",
	)
	if err != nil {
		return nil, errors.Wrap(err, "collect cli handoff manifests")
	}
	if len(manifestErrs) != 0 {
		return nil, errors.Wrap(manifestErrs[0], "collect cli handoff manifest")
	}

	// Select the newest CLI release manifest for each platform.
	byPlatform := make(map[string]*bldr_manifest_world.CollectedManifest, len(cliHandoffPlatformIDs))
	for _, manifest := range collected {
		// Require a complete CLI release manifest before comparing revisions.
		if manifest == nil || manifest.Manifest == nil || manifest.ManifestRef == nil {
			return nil, errors.New("invalid collected cli handoff manifest")
		}
		meta := manifest.Manifest.GetMeta()
		platformID := meta.GetPlatformId()
		if meta.GetManifestId() != cliEntrypointManifestID {
			return nil, errors.New("collected wrong cli handoff manifest id: " + meta.GetManifestId())
		}
		if bldr_manifest.ToBuildType(meta.GetBuildType()) != bldr_manifest.BuildType_RELEASE {
			return nil, errors.New("collected cli handoff manifest has wrong build type: " + meta.GetBuildType())
		}

		// Retain the newest platform revision and reject conflicting frontiers.
		current := byPlatform[platformID]
		if current == nil || manifest.GetRev() > current.GetRev() {
			byPlatform[platformID] = manifest
			continue
		}
		if manifest.GetRev() == current.GetRev() && manifest.ManifestRef.MarshalString() != current.ManifestRef.MarshalString() {
			return nil, errors.New("duplicate cli handoff manifest frontier for platform " + platformID)
		}
	}

	// Build the CLI references in platform order and require a complete matrix.
	out := make([]*cliHandoffManifestRef, 0, len(cliHandoffPlatformIDs))
	var missing []string
	for _, platformID := range cliHandoffPlatformIDs {
		manifest := byPlatform[platformID]
		if manifest == nil {
			missing = append(missing, platformID)
			continue
		}
		out = append(out, &cliHandoffManifestRef{
			ManifestID: cliEntrypointManifestID,
			PlatformID: platformID,
			Rev:        manifest.GetRev(),
			Ref:        manifest.ManifestRef.MarshalString(),
		})
	}
	if len(missing) != 0 {
		return nil, errors.New("missing cli handoff manifest refs for platforms: " + strings.Join(missing, ", "))
	}
	return out, nil
}

func stageCLIHandoffWorld(repoDir, rootDir string) (*entrypointHandoffEntry, error) {
	src := filepath.Join(repoDir, ".bldr", "devtool.s4wave")
	dst := filepath.Join(rootDir, "devtool.s4wave")
	if err := copyFile(src, dst); err != nil {
		return nil, errors.Wrap(err, "stage cli handoff world")
	}
	return buildEntrypointHandoffEntry(rootDir, dst)
}

func stageCLIHandoffArtifacts(rootDir, artifactsDir string) ([]*entrypointHandoffEntry, error) {
	// Copy the CLI archives and collect their handoff entries.
	var out []*entrypointHandoffEntry
	if err := filepath.WalkDir(artifactsDir, func(filePath string, entry os.DirEntry, err error) error {
		// Propagate traversal errors and skip CLI artifact directories.
		if err != nil {
			return errors.Wrap(err, "walk "+filePath)
		}
		if entry.IsDir() {
			return nil
		}

		// Require a regular CLI artifact before copying its contents.
		info, err := entry.Info()
		if err != nil {
			return errors.Wrap(err, "stat "+filePath)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("cli handoff artifact must not be a symlink: " + filePath)
		}
		if !info.Mode().IsRegular() {
			return errors.New("cli handoff artifact is not regular: " + filePath)
		}

		// Resolve the CLI artifact path within the supplied artifact root.
		rel, err := filepath.Rel(artifactsDir, filePath)
		if err != nil {
			return errors.Wrap(err, "rel cli artifact")
		}
		clean := path.Clean(filepath.ToSlash(rel))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") ||
			filepath.IsAbs(rel) || filepath.ToSlash(rel) != clean ||
			strings.Contains(rel, "\\") || strings.Contains(clean, "\\") {
			return errors.New("cli handoff artifact path escapes root: " + filePath)
		}

		// Copy the CLI artifact into the native handoff directory.
		dst := filepath.Join(rootDir, "native", "cli", filepath.FromSlash(clean))
		if err := copyFile(filePath, dst); err != nil {
			return errors.Wrap(err, "stage cli artifact")
		}

		// Record the staged CLI artifact with a path relative to the native root.
		file, err := buildEntrypointHandoffEntry(rootDir, dst)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(file.Path, "native/") {
			return errors.New("cli handoff artifact escaped native root: " + file.Path)
		}
		file.Path = strings.TrimPrefix(file.Path, "native/")
		out = append(out, file)
		return nil
	}); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("cli artifacts are required")
	}

	// Order the CLI artifact entries by their handoff paths.
	slices.SortFunc(out, func(a, b *entrypointHandoffEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

func marshalCLIHandoffManifest(manifest *cliHandoffManifest) string {
	// Write the CLI handoff format and release identity.
	var b strings.Builder
	opts := manifest.opts
	b.WriteString("{\n")
	writeEntrypointJSONField(&b, 1, "format", "cli-handoff.v1", true)
	writeEntrypointJSONField(&b, 1, "release_environment", opts.ReleaseEnvironment, true)
	writeEntrypointJSONField(&b, 1, "cli_rev", opts.CLIRev, true)
	writeEntrypointJSONField(&b, 1, "tag", opts.Tag, true)

	// Write the source provenance of the CLI release.
	writeEntrypointJSONField(&b, 1, "git_sha", opts.GitSHA, true)
	writeEntrypointJSONField(&b, 1, "run_id", opts.RunID, true)
	writeEntrypointJSONField(&b, 1, "run_attempt", opts.RunAttempt, true)
	writeEntrypointJSONField(&b, 1, "source_repo", opts.SourceRepo, true)
	writeEntrypointJSONField(&b, 1, "workflow", opts.Workflow, true)

	// Write the CLI manifest references in their platform order.
	b.WriteString("  \"manifest_refs\": [\n")
	for i, ref := range manifest.manifestRefs {
		// Write the manifest and platform identities for this CLI reference.
		b.WriteString("    {")
		b.WriteString(strconv.Quote("manifest_id"))
		b.WriteString(": ")
		b.WriteString(strconv.Quote(ref.ManifestID))
		b.WriteString(", ")
		b.WriteString(strconv.Quote("platform_id"))
		b.WriteString(": ")
		b.WriteString(strconv.Quote(ref.PlatformID))

		// Write the revision and block reference for this CLI manifest.
		b.WriteString(", ")
		b.WriteString(strconv.Quote("rev"))
		b.WriteString(": ")
		b.WriteString(strconv.FormatUint(ref.Rev, 10))
		b.WriteString(", ")
		b.WriteString(strconv.Quote("ref"))
		b.WriteString(": ")
		b.WriteString(strconv.Quote(ref.Ref))

		// Terminate the CLI reference record and separate it from the next record.
		b.WriteByte('}')
		if i != len(manifest.manifestRefs)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}

	// Write the World entry containing the imported CLI manifests.
	b.WriteString("  ],\n")
	b.WriteString("  \"world\": ")
	writeEntrypointHandoffEntry(&b, manifest.world)
	b.WriteString(",\n")

	// Write the staged CLI artifact entries and close the handoff manifest.
	b.WriteString("  \"artifacts\": [\n")
	for i, artifact := range manifest.artifacts {
		b.WriteString("    ")
		writeEntrypointHandoffEntry(&b, artifact)
		if i != len(manifest.artifacts)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	return b.String()
}
