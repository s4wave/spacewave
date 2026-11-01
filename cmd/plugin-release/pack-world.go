//go:build !js

package main

import (
	"cmp"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	devtool_store "github.com/s4wave/spacewave/bldr/devtool/store"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_pack "github.com/s4wave/spacewave/bldr/manifest/pack"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// worldFlag collects repeated world path flags.
type worldFlag []string

// String returns the paths joined by commas.
func (w *worldFlag) String() string {
	return strings.Join(*w, ",")
}

// Set appends a non-empty path.
func (w *worldFlag) Set(v string) error {
	if v == "" {
		return errors.New("world path cannot be empty")
	}
	*w = append(*w, v)
	return nil
}

type mountedWorld struct {
	vol volume.Volume
	ws  world.WorldState
}

type manifestInventoryEntry struct {
	manifestID string
	platformID string
	rev        uint64
	ref        string
}

func runPackWorld(args []string) error {
	// Declare the pack-world flags.
	fs := flag.NewFlagSet("pack-world", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outDir, refsPath, gitSHA string
	var worldPaths worldFlag
	fs.StringVar(&outDir, "out", "", "directory receiving manifest.pack.kvf and manifest-pack.bin")
	fs.StringVar(&refsPath, "refs-out", "", "path to the output manifest refs JSON")
	fs.StringVar(&gitSHA, "git-sha", "", "source git SHA")
	fs.Var(&worldPaths, "world", "path to an input devtool.s4wave")

	// Parse the arguments.
	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "parse flags")
	}

	// Require every flag before packing.
	if outDir == "" || refsPath == "" || gitSHA == "" || len(worldPaths) == 0 {
		return errors.New(
			"usage: plugin-release pack-world --out /path/to/dir --refs-out /path/to/manifest-refs.json --git-sha SHA --world /path/to/input.s4wave [--world ...]",
		)
	}

	// Pack the input worlds.
	return packWorlds(context.Background(), logrus.NewEntry(logrus.New()), outDir, refsPath, gitSHA, worldPaths)
}

// packWorlds writes the manifests of the input worlds as one release-encoded
// manifest pack in outDir, and their encoded references to refsPath.
func packWorlds(
	ctx context.Context,
	le *logrus.Entry,
	outDir, refsPath, gitSHA string,
	worldPaths []string,
) error {
	// Create the scratch world that receives the release-encoded blocks.
	scratchDir, err := os.MkdirTemp("", "plugin-release-pack-")
	if err != nil {
		return errors.Wrap(err, "create scratch dir")
	}
	defer os.RemoveAll(scratchDir)
	scratch, err := createDevtoolWorld(ctx, le, filepath.Join(scratchDir, "scratch.s4wave"))
	if err != nil {
		return errors.Wrap(err, "create scratch world")
	}
	defer scratch.vol.Close()

	// Collect the manifests of every input world once.
	var sources []bldr_manifest_pack.ReleaseSource
	seen := map[string]struct{}{}
	for _, path := range worldPaths {
		src, err := openDevtoolWorld(ctx, le, path)
		if err != nil {
			return errors.Wrapf(err, "open input world %s", path)
		}
		defer src.vol.Close()
		collected, err := collectWorldSources(ctx, src.ws, seen)
		if err != nil {
			return errors.Wrapf(err, "collect input world %s", path)
		}
		sources = append(sources, collected...)
	}
	if len(sources) == 0 {
		return errors.New("no manifests to pack")
	}

	// Open the pack file in the output directory.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir output dir")
	}
	packFile, err := os.Create(filepath.Join(outDir, bldr_manifest_pack.ArtifactPackFilename))
	if err != nil {
		return errors.Wrap(err, "create pack")
	}
	defer packFile.Close()

	// Write the release pack and its metadata.
	meta, refs, err := bldr_manifest_pack.ProduceReleasePack(ctx, le, &bldr_manifest_pack.ReleasePackConfig{
		WorldState:     scratch.ws,
		Sender:         scratch.vol.GetPeerID(),
		BundleKey:      devtool_store.PluginHostObjectKey,
		Sources:        sources,
		GitSHA:         gitSHA,
		ProducerTarget: "plugin-release",
		CacheSchema:    "manifest-pack-v1",
		Writer:         packFile,
	})
	if err != nil {
		return errors.Wrap(err, "produce release pack")
	}
	if err := packFile.Close(); err != nil {
		return errors.Wrap(err, "close pack")
	}
	metaData, err := meta.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal pack metadata")
	}
	if err := os.WriteFile(filepath.Join(outDir, bldr_manifest_pack.ArtifactMetadataFilename), metaData, 0o644); err != nil {
		return errors.Wrap(err, "write pack metadata")
	}

	// Record the encoded manifest references for the handoff.
	return os.WriteFile(refsPath, []byte(marshalManifestInventory(manifestInventory(refs))), 0o644)
}

// collectWorldSources returns the manifests of a world that seen does not hold,
// and marks them seen. The same manifest built on two hosts is packed once.
func collectWorldSources(
	ctx context.Context,
	ws world.WorldState,
	seen map[string]struct{},
) ([]bldr_manifest_pack.ReleaseSource, error) {
	// Collect the manifests stored under the devtool plugin host.
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, devtool_store.PluginHostObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifests")
	}
	if len(manifestErrs) != 0 {
		return nil, errors.Wrap(manifestErrs[0], "collect manifest")
	}

	// Keep each manifest identity the first time it appears.
	var out []bldr_manifest_pack.ReleaseSource
	for _, list := range manifests {
		for _, manifest := range list {
			meta := manifest.Manifest.GetMeta()
			key := meta.GetManifestId() + "\x00" + meta.GetPlatformId() + "\x00" + strconv.FormatUint(meta.GetRev(), 10)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, bldr_manifest_pack.ReleaseSource{Access: ws.AccessWorldState, Ref: manifest.ManifestRef})
		}
	}
	return out, nil
}

// openDevtoolWorld mounts an existing devtool world read-only.
func openDevtoolWorld(ctx context.Context, le *logrus.Entry, path string) (*mountedWorld, error) {
	return openMountedWorld(ctx, le, path, true)
}

// createDevtoolWorld creates an empty devtool world at path, replacing any
// file there.
func createDevtoolWorld(ctx context.Context, le *logrus.Entry, path string) (*mountedWorld, error) {
	// Clear the path and its directory for a fresh world.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "remove output world")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "mkdir output dir")
	}

	// Mount the new world.
	return openMountedWorld(ctx, le, path, false)
}

// openMountedWorld opens the devtool database at path and mounts its World.
// An existing World is opened without writing its key; otherwise the World is
// created.
func openMountedWorld(ctx context.Context, le *logrus.Entry, path string, existing bool) (*mountedWorld, error) {
	// Open the volume.
	vol, err := devtool_store.OpenVolumeFile(ctx, le, path, !existing)
	if err != nil {
		return nil, err
	}

	// Mount the devtool World over the volume.
	eng, err := devtool_store.OpenWorld(ctx, le, vol, !existing)
	if err != nil {
		_ = vol.Close()
		return nil, err
	}
	return &mountedWorld{vol: vol, ws: world.NewEngineWorldState(eng, true)}, nil
}

// manifestInventory lists the encoded manifest references in a stable order.
func manifestInventory(refs []*bldr_manifest.ManifestRef) []manifestInventoryEntry {
	// Describe each reference by identity and encoded reference.
	out := make([]manifestInventoryEntry, 0, len(refs))
	for _, ref := range refs {
		meta := ref.GetMeta()
		out = append(out, manifestInventoryEntry{
			manifestID: meta.GetManifestId(),
			platformID: meta.GetPlatformId(),
			rev:        meta.GetRev(),
			ref:        ref.GetManifestRef().MarshalString(),
		})
	}

	// Sort by manifest, platform, then revision.
	slices.SortFunc(out, func(a, b manifestInventoryEntry) int {
		if c := strings.Compare(a.manifestID, b.manifestID); c != 0 {
			return c
		}
		if c := strings.Compare(a.platformID, b.platformID); c != 0 {
			return c
		}
		return cmp.Compare(a.rev, b.rev)
	})
	return out
}

// marshalManifestInventory renders the inventory as a JSON array.
func marshalManifestInventory(entries []manifestInventoryEntry) string {
	// Open the array.
	var sb strings.Builder
	sb.WriteString("[\n")

	// Write one object per entry.
	for i, entry := range entries {
		if i != 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString("  {")
		sb.WriteString("\"manifest_id\":")
		sb.WriteString(strconv.Quote(entry.manifestID))
		sb.WriteString(",\"platform_id\":")
		sb.WriteString(strconv.Quote(entry.platformID))
		sb.WriteString(",\"rev\":")
		sb.WriteString(strconv.FormatUint(entry.rev, 10))
		sb.WriteString(",\"ref\":")
		sb.WriteString(strconv.Quote(entry.ref))
		sb.WriteString("}")
	}

	// Close the array.
	sb.WriteString("\n]\n")
	return sb.String()
}
