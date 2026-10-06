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

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_pack "github.com/s4wave/spacewave/bldr/manifest/pack"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	devtoolEngineBucketID = "bldr/devtool"
	devtoolPluginHostKey  = "devtool"
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
		BundleKey:      devtoolPluginHostKey,
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
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, devtoolPluginHostKey)
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

// openMountedWorld opens the s4db volume at path and mounts its devtool world.
// An existing world is opened without writing its key; otherwise the world is
// created.
func openMountedWorld(ctx context.Context, le *logrus.Entry, path string, existing bool) (*mountedWorld, error) {
	// Require an existing world file before opening it.
	if existing {
		if _, err := os.Stat(path); err != nil {
			return nil, errors.Wrap(err, "stat world")
		}
	}

	// Open the volume.
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:          path,
		NoGenerateKey: existing,
		NoWriteKey:    existing,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open volume")
	}

	// Mount the devtool world over the volume.
	mw, err := mountDevtoolWorld(ctx, le, vol, !existing)
	if err != nil {
		_ = vol.Close()
		return nil, err
	}
	return mw, nil
}

func mountDevtoolWorld(ctx context.Context, le *logrus.Entry, vol volume.Volume, create bool) (*mountedWorld, error) {
	// Register the gzip step and build the world transform config.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	// Choose the head reference: a new one, or the stored one.
	var headRef *bucket.ObjectRef
	if create {
		headRef = &bucket.ObjectRef{
			BucketId:      devtoolEngineBucketID,
			TransformConf: transformConf,
		}
	} else {
		headRef, err = loadWorldHeadRef(ctx, vol)
		if err != nil {
			return nil, errors.Wrap(err, "load head ref")
		}
		if headRef.GetRootRef().GetEmpty() {
			return nil, errors.New("devtool world is empty")
		}
		if headRef.GetBucketId() == "" {
			headRef.BucketId = devtoolEngineBucketID
		}
	}

	// Build the block transformer and the bucket cursor over the head.
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build block transformer")
	}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		sfs,
		vol,
		xfrm,
		headRef,
		&bucket.BucketOpArgs{BucketId: devtoolEngineBucketID},
		transformConf,
	)

	// Write the initial empty World for a new volume.
	if create {
		btx, bcs := cursor.BuildTransaction(nil)
		bcs.ClearAllRefs()
		bcs.SetBlock(world_block.NewWorld(false), true)
		rootRef, _, err := btx.Write(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "write initial world")
		}
		headRef.RootRef = rootRef
		cursor.SetRootRef(rootRef)
		if err := writeWorldHeadRef(ctx, vol, headRef); err != nil {
			return nil, errors.Wrap(err, "write head ref")
		}
	}

	// Build the world engine that persists each head to the volume.
	commitFn := func(ctx context.Context, _ *bucket.ObjectRef, ref *bucket.ObjectRef) error {
		return writeWorldHeadRef(ctx, vol, ref)
	}
	eng, err := world_block.NewEngine(ctx, le, cursor, bldr_manifest_world.LookupOp, commitFn, false)
	if err != nil {
		return nil, errors.Wrap(err, "build world engine")
	}
	ws := world.NewEngineWorldState(eng, true)

	// Create the plugin host's manifest store in a new world.
	if create {
		if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, devtoolPluginHostKey); err != nil {
			return nil, errors.Wrap(err, "create manifest store")
		}
	}
	return &mountedWorld{vol: vol, ws: ws}, nil
}

func loadWorldHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
	// Open a read transaction on the engine's object store.
	store, rel, err := vol.AccessObjectStore(ctx, devtoolEngineBucketID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access object store")
	}
	defer rel()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open object store tx")
	}
	defer tx.Discard()

	// Read the stored head state.
	data, found, err := tx.Get(ctx, []byte("world-head"))
	if err != nil {
		return nil, errors.Wrap(err, "read world-head")
	}
	if !found {
		return nil, errors.Errorf("world-head not found in object store %s", devtoolEngineBucketID)
	}

	// Decode the head reference.
	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}

func writeWorldHeadRef(ctx context.Context, vol volume.Volume, ref *bucket.ObjectRef) error {
	// Open a write transaction on the engine's object store.
	store, rel, err := vol.AccessObjectStore(ctx, devtoolEngineBucketID, nil)
	if err != nil {
		return errors.Wrap(err, "access object store")
	}
	defer rel()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open object store tx")
	}
	defer tx.Discard()

	// Store the encoded head state.
	state := &world_block_engine.HeadState{HeadRef: ref}
	data, err := state.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal head state")
	}
	if err := tx.Set(ctx, []byte("world-head"), data); err != nil {
		return errors.Wrap(err, "write world-head")
	}
	return tx.Commit(ctx)
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
