//go:build !js

package main

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const (
	devtoolEngineBucketID = "bldr/devtool"
	devtoolPluginHostKey  = "devtool"
)

type worldFlag []string

func (w *worldFlag) String() string {
	return strings.Join(*w, ",")
}

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

func runConsolidateWorld(args []string) error {
	fs := flag.NewFlagSet("consolidate-world", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var outPath string
	var worldPaths worldFlag
	if err := func() error {
		fs.StringVar(&outPath, "out", "", "path to the output devtool.s4wave")
		fs.Var(&worldPaths, "world", "path to an input devtool.s4wave")
		return fs.Parse(args)
	}(); err != nil {
		return errors.Wrap(err, "parse flags")
	}
	if outPath == "" || len(worldPaths) == 0 {
		return errors.New(
			"usage: plugin-release consolidate-world --out /path/to/devtool.s4wave --world /path/to/input.s4wave [--world ...]",
		)
	}

	log := logrus.New()
	le := logrus.NewEntry(log)
	ctx := context.Background()

	out, err := createDevtoolWorld(ctx, le, outPath)
	if err != nil {
		return errors.Wrap(err, "create output world")
	}
	defer out.vol.Close()

	ts := timestamp.Now()
	seen := map[string]struct{}{}
	for _, path := range worldPaths {
		src, err := openDevtoolWorld(ctx, le, path)
		if err != nil {
			return errors.Wrapf(err, "open input world %s", path)
		}
		if err := copyWorldManifests(ctx, le, src.ws, out.ws, out.vol.GetPeerID(), ts, seen); err != nil {
			_ = src.vol.Close()
			return errors.Wrapf(err, "copy input world %s", path)
		}
		if err := src.vol.Close(); err != nil {
			return errors.Wrapf(err, "close input world %s", path)
		}
	}

	if len(seen) == 0 {
		return errors.New("no manifests copied")
	}
	return nil
}

func openDevtoolWorld(ctx context.Context, le *logrus.Entry, path string) (*mountedWorld, error) {
	vol, err := openBoltVolume(ctx, le, path, true)
	if err != nil {
		return nil, err
	}
	mw, err := mountDevtoolWorld(ctx, le, vol, false)
	if err != nil {
		_ = vol.Close()
		return nil, err
	}
	return mw, nil
}

func createDevtoolWorld(ctx context.Context, le *logrus.Entry, path string) (*mountedWorld, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "remove output world")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "mkdir output dir")
	}

	vol, err := openBoltVolume(ctx, le, path, false)
	if err != nil {
		return nil, err
	}
	mw, err := mountDevtoolWorld(ctx, le, vol, true)
	if err != nil {
		_ = vol.Close()
		return nil, err
	}
	return mw, nil
}

func openBoltVolume(ctx context.Context, le *logrus.Entry, path string, existing bool) (volume.Volume, error) {
	if existing {
		if _, err := os.Stat(path); err != nil {
			return nil, errors.Wrap(err, "stat world")
		}
	}
	conf := &volume_bolt.Config{
		Path:          path,
		NoGenerateKey: existing,
		NoWriteKey:    existing,
	}
	vol, err := volume_bolt.NewBolt(ctx, le, conf)
	if err != nil {
		return nil, errors.Wrap(err, "open bolt volume")
	}
	return vol, nil
}

func mountDevtoolWorld(ctx context.Context, le *logrus.Entry, vol volume.Volume, create bool) (*mountedWorld, error) {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

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

	commitFn := func(ref *bucket.ObjectRef) error {
		return writeWorldHeadRef(ctx, vol, ref)
	}
	eng, err := world_block.NewEngine(ctx, le, cursor, bldr_manifest_world.LookupOp, commitFn, false)
	if err != nil {
		return nil, errors.Wrap(err, "build world engine")
	}
	ws := world.NewEngineWorldState(eng, true)
	if create {
		if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, devtoolPluginHostKey); err != nil {
			return nil, errors.Wrap(err, "create manifest store")
		}
	}
	return &mountedWorld{vol: vol, ws: ws}, nil
}

func loadWorldHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
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

	data, found, err := tx.Get(ctx, []byte("world-head"))
	if err != nil {
		return nil, errors.Wrap(err, "read world-head")
	}
	if !found {
		return nil, errors.Errorf("world-head not found in object store %s", devtoolEngineBucketID)
	}

	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}

func writeWorldHeadRef(ctx context.Context, vol volume.Volume, ref *bucket.ObjectRef) error {
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

func copyWorldManifests(
	ctx context.Context,
	le *logrus.Entry,
	src world.WorldState,
	dest world.WorldState,
	destPeer peer.ID,
	ts *timestamp.Timestamp,
	seen map[string]struct{},
) error {
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, src, nil, devtoolPluginHostKey)
	if err != nil {
		return errors.Wrap(err, "collect manifests")
	}
	if len(manifestErrs) != 0 {
		return errors.Wrap(manifestErrs[0], "collect manifest")
	}

	for _, list := range manifests {
		for _, manifest := range list {
			meta := manifest.Manifest.GetMeta()
			key := meta.GetManifestId() + "\x00" + meta.GetPlatformId() + "\x00" + strconv.FormatUint(meta.GetRev(), 10)
			if _, ok := seen[key]; ok {
				continue
			}
			_, _, err := bldr_manifest_world.DeepCopyManifest(
				ctx,
				le,
				src.AccessWorldState,
				manifest.ManifestRef,
				dest,
				dest.AccessWorldState,
				manifest.ManifestKey,
				[]string{devtoolPluginHostKey},
				destPeer,
				ts,
			)
			if err != nil {
				return errors.Wrapf(err, "copy manifest %s/%s rev %d", meta.GetManifestId(), meta.GetPlatformId(), meta.GetRev())
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}
