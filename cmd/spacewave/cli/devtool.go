//go:build !js

package spacewave_cli

import (
	"bytes"
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	// devtoolEngineBucketID is the bucket ID used by the devtool world engine.
	devtoolEngineBucketID = "bldr/devtool"
	// devtoolEngineObjStoreID is the object store ID used by the devtool world engine.
	devtoolEngineObjStoreID = "bldr/devtool"
	// devtoolPluginHostObjectKey is the object key for the devtool plugin host.
	devtoolPluginHostObjectKey = "devtool"
)

// openDevtoolVolume opens the devtool bolt volume at the given .bldr/ path.
func openDevtoolVolume(ctx context.Context, le *logrus.Entry, bldrPath string) (volume.Volume, error) {
	dbPath := filepath.Join(bldrPath, "devtool.s4wave")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, errors.Errorf("devtool database not found at %s", dbPath)
	}
	conf := &volume_bolt.Config{
		Path:          dbPath,
		NoGenerateKey: true,
		NoWriteKey:    true,
	}
	return volume_bolt.NewBolt(ctx, le, conf)
}

// buildStepFactorySet builds the block transform step factory set with gzip support.
func buildStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	return sfs
}

// loadDevtoolHeadRef reads the world engine head ref from the volume's object store.
func loadDevtoolHeadRef(ctx context.Context, vol volume.Volume) (*bucket.ObjectRef, error) {
	store, rel, err := vol.AccessObjectStore(ctx, devtoolEngineObjStoreID, nil)
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
		return nil, errors.Errorf("world-head not found in object store %s", devtoolEngineObjStoreID)
	}

	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal head state")
	}
	return state.GetHeadRef(), nil
}

func openDevtoolWorldEngine(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
) (*devtoolWorldEngine, error) {
	headRef, err := loadDevtoolHeadRef(ctx, vol)
	if err != nil {
		return nil, errors.Wrap(err, "load head ref")
	}
	if headRef.GetRootRef().GetEmpty() {
		return nil, errors.New("devtool world is empty (no head ref)")
	}

	if headRef.GetBucketId() == "" {
		headRef.BucketId = devtoolEngineBucketID
	}

	sfs := buildStepFactorySet()

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
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
		&bucket.BucketOpArgs{
			BucketId: devtoolEngineBucketID,
		},
		transformConf,
	)

	store, rel, err := vol.AccessObjectStore(ctx, devtoolEngineObjStoreID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access object store")
	}

	commitFn := func(ctx context.Context, _ *bucket.ObjectRef, nref *bucket.ObjectRef) error {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open object store tx")
		}
		defer tx.Discard()
		state := &world_block_engine.HeadState{HeadRef: nref}
		data, err := state.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal head state")
		}
		if err := tx.Set(ctx, []byte("world-head"), data); err != nil {
			return errors.Wrap(err, "write world-head")
		}
		return tx.Commit(ctx)
	}

	eng, err := world_block.NewEngine(
		ctx,
		le,
		cursor,
		bldr_manifest_world.LookupOp,
		commitFn,
		false,
	)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "build world engine")
	}

	return &devtoolWorldEngine{Engine: eng, release: rel}, nil
}

type devtoolWorldEngine struct {
	world.Engine
	release func()
}

func (e *devtoolWorldEngine) Close() error {
	e.release()
	return nil
}

// collectLatestManifestSet returns one latest manifest reference per platform.
//
// It rejects unavailable or invalid candidates and ambiguous equal-revision
// references instead of selecting one by traversal order. Returned references
// are sorted by manifest ID, descending revision, platform ID, and reference.
func collectLatestManifestSet(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
) ([]*bldr_manifest.ManifestRef, error) {
	if err := bldr_manifest.ValidateManifestID(manifestID, false); err != nil {
		return nil, err
	}
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, devtoolPluginHostObjectKey)
	if err != nil {
		return nil, err
	}
	if len(manifestErrs) != 0 {
		return nil, errors.Wrap(manifestErrs[0], "collect manifest set")
	}
	candidates := manifests[manifestID]
	if len(candidates) == 0 {
		return nil, errors.Errorf("manifest %q not found", manifestID)
	}
	selected := make(map[string]*bldr_manifest.ManifestRef)
	for _, candidate := range candidates {
		meta := candidate.Manifest.GetMeta()
		if err := candidate.Manifest.Validate(); err != nil {
			return nil, errors.Wrapf(err, "manifest %s", candidate.ManifestKey)
		}
		if _, err := bldr_platform.ParsePlatform(meta.GetPlatformId()); err != nil {
			return nil, errors.Wrapf(err, "manifest %s platform", candidate.ManifestKey)
		}
		canonicalRef, err := bldr_manifest_world.CanonicalizeManifestObjectRef(ctx, ws.AccessWorldState, candidate.ManifestRef)
		if err != nil {
			return nil, errors.Wrapf(err, "manifest %s reference", candidate.ManifestKey)
		}
		if canonicalRef.GetRootRef().GetEmpty() {
			return nil, errors.Errorf("manifest %s has empty root reference", candidate.ManifestKey)
		}
		ref := bldr_manifest.NewManifestRef(meta, canonicalRef)
		if err := ref.Validate(); err != nil {
			return nil, errors.Wrapf(err, "manifest %s", candidate.ManifestKey)
		}
		platformID := meta.GetPlatformId()
		current := selected[platformID]
		if current == nil || meta.GetRev() > current.GetMeta().GetRev() {
			selected[platformID] = ref
			continue
		}
		if meta.GetRev() != current.GetMeta().GetRev() {
			continue
		}
		if !meta.EqualVT(current.GetMeta()) || !canonicalRef.EqualsRef(current.GetManifestRef()) {
			return nil, errors.Errorf("manifest %q platform %q revision %d has ambiguous references", manifestID, platformID, meta.GetRev())
		}
	}
	if len(selected) == 0 {
		return nil, errors.Errorf("manifest %q has no valid platforms", manifestID)
	}
	result := make([]*bldr_manifest.ManifestRef, 0, len(selected))
	for _, ref := range selected {
		result = append(result, ref)
	}
	slices.SortStableFunc(result, func(a, b *bldr_manifest.ManifestRef) int {
		if c := cmp.Compare(a.GetMeta().GetManifestId(), b.GetMeta().GetManifestId()); c != 0 {
			return c
		}
		if c := cmp.Compare(b.GetMeta().GetRev(), a.GetMeta().GetRev()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.GetMeta().GetPlatformId(), b.GetMeta().GetPlatformId()); c != 0 {
			return c
		}
		aBytes, _ := a.GetManifestRef().MarshalVT()
		bBytes, _ := b.GetManifestRef().MarshalVT()
		return bytes.Compare(aBytes, bBytes)
	})
	return result, nil
}

// lookupDevtoolManifestSet opens the devtool world and collects one latest
// manifest reference for every valid platform.
func lookupDevtoolManifestSet(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	manifestID string,
) ([]*bldr_manifest.ManifestRef, error) {
	eng, err := openDevtoolWorldEngine(ctx, le, vol)
	if err != nil {
		return nil, err
	}
	defer eng.Close()
	ws := world.NewEngineWorldState(eng, false)
	refs, err := collectLatestManifestSet(ctx, ws, manifestID)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifest set")
	}
	return refs, nil
}

// lookupDevtoolManifests opens the devtool world and finds latest manifests by ID.
func lookupDevtoolManifests(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
	manifestID string,
) ([]*bldr_manifest_world.CollectedManifest, error) {
	eng, err := openDevtoolWorldEngine(ctx, le, vol)
	if err != nil {
		return nil, err
	}
	defer eng.Close()

	ws := world.NewEngineWorldState(eng, false)

	manifests, _, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, devtoolPluginHostObjectKey)
	if err != nil {
		return nil, errors.Wrap(err, "collect manifests")
	}

	list, ok := manifests[manifestID]
	if !ok || len(list) == 0 {
		available := make([]string, 0, len(manifests))
		for id := range manifests {
			available = append(available, id)
		}
		return nil, errors.Errorf("manifest %q not found (available: %v)", manifestID, available)
	}

	list = bldr_manifest_world.FilterCollectedManifestsByLatestRev(list)
	return list, nil
}
